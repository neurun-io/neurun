#!/usr/bin/env python3
"""Validate user-application descriptor drift and workflow graph semantics."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import PurePosixPath
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--application", required=True)
    parser.add_argument("--workflow", required=True)
    parser.add_argument("--expected-workflow", required=True)
    parser.add_argument("--expected-container-context", required=True)
    parser.add_argument("--expected-container-file", required=True)
    parser.add_argument("--expected-artifact-directory", required=True)
    return parser.parse_args()


def load_json(path: str, label: str, errors: list[str]) -> Any:
    try:
        with open(path, encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        errors.append(f"{label}: cannot read valid JSON from {path!r}: {error}")
        return None


def is_repository_relative(value: Any) -> bool:
    if (
        not isinstance(value, str)
        or not re.fullmatch(r"[A-Za-z0-9._/-]+", value)
        or ".." in value
    ):
        return False
    path = PurePosixPath(value)
    return not path.is_absolute() and all(part not in ("", "..") for part in path.parts)


def nested_value(document: Any, keys: tuple[str, ...]) -> Any:
    current = document
    for key in keys:
        if not isinstance(current, dict) or key not in current:
            return None
        current = current[key]
    return current


def validate_object_shape(
    document: Any,
    label: str,
    required: set[str],
    allowed: set[str],
    errors: list[str],
) -> bool:
    if not isinstance(document, dict):
        errors.append(f"{label}: must be a JSON object")
        return False
    for key in sorted(required - document.keys()):
        errors.append(f"{label}.{key}: required property is missing")
    for key in sorted(document.keys() - allowed):
        errors.append(f"{label}.{key}: property is not allowed")
    return True


def is_json_integer(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def validate_application_contract(application: Any, errors: list[str]) -> None:
    allowed = {
        "schema_version",
        "name",
        "workflow",
        "container",
        "artifacts",
        "environments",
    }
    if not validate_object_shape(application, "application", allowed, allowed, errors):
        return

    if application.get("schema_version") != "1":
        errors.append("application.schema_version: must equal '1'")

    name = application.get("name")
    if not isinstance(name, str) or not re.fullmatch(r"[a-z][a-z0-9-]{0,62}", name):
        errors.append("application.name: must match ^[a-z][a-z0-9-]{0,62}$")

    workflow = application.get("workflow")
    if (
        not is_repository_relative(workflow)
        or not workflow.endswith(".json")
        or len(workflow) > 256
    ):
        errors.append(
            "application.workflow: must be a repository-relative .json path "
            "of at most 256 characters"
        )

    container = application.get("container")
    if validate_object_shape(
        container,
        "application.container",
        {"context", "dockerfile"},
        {"context", "dockerfile"},
        errors,
    ):
        for field in ("context", "dockerfile"):
            value = container.get(field)
            if not is_repository_relative(value) or len(value) > 256:
                errors.append(
                    f"application.container.{field}: must be a safe "
                    "repository-relative path of at most 256 characters"
                )

    artifacts = application.get("artifacts")
    if validate_object_shape(
        artifacts,
        "application.artifacts",
        {"directory"},
        {"directory"},
        errors,
    ):
        directory = artifacts.get("directory")
        if not is_repository_relative(directory) or len(directory) > 256:
            errors.append(
                "application.artifacts.directory: must be a safe "
                "repository-relative path of at most 256 characters"
            )

    environments = application.get("environments")
    if not isinstance(environments, list) or not environments:
        errors.append("application.environments: must be a non-empty array")
    elif not all(
        isinstance(item, str)
        and re.fullmatch(r"[a-z][a-z0-9-]{0,62}", item)
        for item in environments
    ):
        errors.append(
            "application.environments: each value must be a lowercase "
            "environment name"
        )
    elif len(set(environments)) != len(environments):
        errors.append("application.environments: values must be unique")


def validate_workflow_contract(workflow: Any, errors: list[str]) -> None:
    if not validate_object_shape(
        workflow,
        "workflow",
        {"version", "steps"},
        {"version", "name", "timeout_ms", "steps"},
        errors,
    ):
        return

    if workflow.get("version") != "1":
        errors.append("workflow.version: must equal '1'")

    if "name" in workflow:
        name = workflow["name"]
        if not isinstance(name, str) or not 1 <= len(name) <= 128:
            errors.append("workflow.name: must contain 1 to 128 characters")

    if "timeout_ms" in workflow:
        timeout = workflow["timeout_ms"]
        if not is_json_integer(timeout) or not 1 <= timeout <= 3_600_000:
            errors.append("workflow.timeout_ms: must be an integer from 1 to 3600000")

    steps = workflow.get("steps")
    if not isinstance(steps, list) or not 1 <= len(steps) <= 256:
        errors.append("workflow.steps: must contain 1 to 256 steps")
        return

    for index, step in enumerate(steps):
        label = f"workflow.steps[{index}]"
        if not validate_object_shape(
            step,
            label,
            {"id", "function", "input"},
            {"id", "function", "depends_on", "input", "timeout_ms", "when"},
            errors,
        ):
            continue

        step_id = step.get("id")
        if not isinstance(step_id, str) or not re.fullmatch(
            r"[a-z][a-z0-9_]{0,63}", step_id
        ):
            errors.append(f"{label}.id: must be a valid lowercase step id")

        function = step.get("function")
        if validate_object_shape(
            function,
            f"{label}.function",
            {"name", "version"},
            {"name", "version", "digest"},
            errors,
        ):
            function_name = function.get("name")
            if not isinstance(function_name, str) or not re.fullmatch(
                r"[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+", function_name
            ):
                errors.append(
                    f"{label}.function.name: must be a dotted lowercase name"
                )
            version = function.get("version")
            if not isinstance(version, str) or not version:
                errors.append(f"{label}.function.version: must be a non-empty string")
            digest = function.get("digest")
            if digest is not None and (
                not isinstance(digest, str)
                or not re.fullmatch(r"sha256:[0-9a-f]{64}", digest)
            ):
                errors.append(f"{label}.function.digest: must be a sha256 digest")

        if not isinstance(step.get("input"), dict):
            errors.append(f"{label}.input: must be a JSON object")

        if "depends_on" in step:
            dependencies = step["depends_on"]
            if not isinstance(dependencies, list) or not all(
                isinstance(item, str) for item in dependencies
            ):
                errors.append(f"{label}.depends_on: must contain only strings")
            elif len(set(dependencies)) != len(dependencies):
                errors.append(f"{label}.depends_on: values must be unique")

        if "timeout_ms" in step:
            timeout = step["timeout_ms"]
            if not is_json_integer(timeout) or timeout < 1:
                errors.append(f"{label}.timeout_ms: must be a positive integer")

        if "when" in step:
            condition = step["when"]
            if not isinstance(condition, str) or len(condition) > 1024:
                errors.append(f"{label}.when: must contain at most 1024 characters")


def validate_descriptor(
    application: Any,
    expected: dict[tuple[str, ...], str],
    errors: list[str],
) -> None:
    validate_application_contract(application, errors)
    if not isinstance(application, dict):
        return

    path_fields = (
        ("workflow",),
        ("container", "context"),
        ("container", "dockerfile"),
        ("artifacts", "directory"),
    )
    for keys in path_fields:
        value = nested_value(application, keys)
        label = "application." + ".".join(keys)
        if not is_repository_relative(value):
            errors.append(f"{label}: must be a safe repository-relative path")

    for keys, expected_value in expected.items():
        actual = nested_value(application, keys)
        label = "application." + ".".join(keys)
        if actual != expected_value:
            errors.append(
                f"{label}: descriptor value {actual!r} does not match "
                f"caller input {expected_value!r}"
            )


def validate_workflow_graph(workflow: Any, errors: list[str]) -> None:
    validate_workflow_contract(workflow, errors)
    if not isinstance(workflow, dict):
        return
    steps = workflow.get("steps")
    if not isinstance(steps, list):
        return

    dependencies: dict[str, list[str]] = {}
    for index, step in enumerate(steps):
        if not isinstance(step, dict):
            errors.append(f"workflow.steps[{index}]: must be an object")
            continue
        step_id = step.get("id")
        if not isinstance(step_id, str) or not step_id:
            errors.append(f"workflow.steps[{index}].id: must be a non-empty string")
            continue
        if step_id in dependencies:
            errors.append(f"workflow.steps[{index}].id: duplicate step id {step_id!r}")
            continue

        raw_dependencies = step.get("depends_on", [])
        if not isinstance(raw_dependencies, list) or not all(
            isinstance(item, str) for item in raw_dependencies
        ):
            errors.append(
                f"workflow.steps[{index}].depends_on: must contain only step ids"
            )
            raw_dependencies = []
        dependencies[step_id] = list(raw_dependencies)

    for step_id, required_steps in dependencies.items():
        for required_step in required_steps:
            if required_step not in dependencies:
                errors.append(
                    f"workflow step {step_id!r}: unknown dependency {required_step!r}"
                )

    state: dict[str, int] = {}
    stack: list[str] = []

    def visit(step_id: str) -> None:
        if state.get(step_id) == 2:
            return
        if state.get(step_id) == 1:
            try:
                cycle_start = stack.index(step_id)
            except ValueError:
                cycle_start = 0
            cycle = stack[cycle_start:] + [step_id]
            errors.append("workflow: dependency cycle: " + " -> ".join(cycle))
            return

        state[step_id] = 1
        stack.append(step_id)
        for dependency in dependencies.get(step_id, []):
            if dependency in dependencies:
                visit(dependency)
        stack.pop()
        state[step_id] = 2

    for step_id in dependencies:
        if state.get(step_id, 0) == 0:
            visit(step_id)


def github_error(message: str) -> str:
    escaped = (
        message.replace("%", "%25")
        .replace("\r", "%0D")
        .replace("\n", "%0A")
    )
    return f"::error title=Invalid Neurun user application::{escaped}"


def main() -> int:
    args = parse_args()
    errors: list[str] = []
    application = load_json(args.application, "application", errors)
    workflow = load_json(args.workflow, "workflow", errors)

    validate_descriptor(
        application,
        {
            ("workflow",): args.expected_workflow,
            ("container", "context"): args.expected_container_context,
            ("container", "dockerfile"): args.expected_container_file,
            ("artifacts", "directory"): args.expected_artifact_directory,
        },
        errors,
    )
    validate_workflow_graph(workflow, errors)

    if errors:
        for error in errors:
            print(github_error(error), file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
