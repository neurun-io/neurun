"""Regression tests for the dependency-free user-application validator."""

from __future__ import annotations

import copy
import json
import runpy
import unittest
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
VALIDATOR = runpy.run_path(str(ROOT / "scripts" / "validate-user-app.py"))


def load_json(relative_path: str) -> Any:
    return json.loads((ROOT / relative_path).read_text(encoding="utf-8"))


class UserApplicationValidatorTests(unittest.TestCase):
    def setUp(self) -> None:
        self.application = load_json(
            "templates/user-app/neurun/application.json"
        )
        self.workflow = load_json("templates/user-app/neurun/workflow.json")

    def workflow_errors(self, workflow: dict[str, Any]) -> list[str]:
        errors: list[str] = []
        VALIDATOR["validate_workflow_graph"](workflow, errors)
        return errors

    def test_shipped_templates_are_valid(self) -> None:
        errors: list[str] = []
        VALIDATOR["validate_descriptor"](
            self.application,
            {
                ("workflow",): "neurun/workflow.json",
                ("container", "context"): ".",
                ("container", "dockerfile"): "Dockerfile",
                ("artifacts", "directory"): "dist",
            },
            errors,
        )
        VALIDATOR["validate_workflow_graph"](self.workflow, errors)
        self.assertEqual(errors, [])

    def test_duplicate_step_ids_are_rejected(self) -> None:
        workflow = copy.deepcopy(self.workflow)
        workflow["steps"].append(copy.deepcopy(workflow["steps"][0]))
        self.assertTrue(
            any("duplicate step id" in error for error in self.workflow_errors(workflow))
        )

    def test_unknown_dependencies_are_rejected(self) -> None:
        workflow = copy.deepcopy(self.workflow)
        workflow["steps"][0]["depends_on"] = ["missing"]
        self.assertTrue(
            any("unknown dependency" in error for error in self.workflow_errors(workflow))
        )

    def test_dependency_cycles_are_rejected(self) -> None:
        workflow = copy.deepcopy(self.workflow)
        first = workflow["steps"][0]
        first["depends_on"] = ["second"]
        workflow["steps"].append(
            {
                "id": "second",
                "function": {"name": "system.echo", "version": "1"},
                "depends_on": [first["id"]],
                "input": {},
            }
        )
        self.assertTrue(
            any("dependency cycle" in error for error in self.workflow_errors(workflow))
        )

    def test_absolute_descriptor_paths_are_rejected(self) -> None:
        application = copy.deepcopy(self.application)
        application["container"]["context"] = "/tmp/application"
        errors: list[str] = []
        VALIDATOR["validate_application_contract"](application, errors)
        self.assertTrue(
            any("container.context" in error for error in errors),
            errors,
        )

    def test_descriptor_and_caller_inputs_cannot_drift(self) -> None:
        errors: list[str] = []
        VALIDATOR["validate_descriptor"](
            self.application,
            {("artifacts", "directory"): "build"},
            errors,
        )
        self.assertTrue(
            any("does not match caller input" in error for error in errors),
            errors,
        )


if __name__ == "__main__":
    unittest.main()
