# Concepts

Ten objects. Read them in this order — each one only makes sense on top of
the last.

| Concept | One line |
| --- | --- |
| [Project](project.md) | A namespace that owns apps and everything under them. |
| [App](app.md) | A named thing you deploy to. Must exist before a deploy. |
| [Deployment](deployment.md) | One attempt at turning a commit into a build. |
| [Build](build.md) | What a deployment produced, and what an app runs. |
| [Artifact](artifact.md) | One layer of a build, stored as an immutable blob. |
| [Execution](execution.md) | One invocation of an app. The billable unit. |
| [Browser profile](browser-profile.md) | Who a browser appears to be, and what it remembers. |
| [Browser session](browser-session.md) | A browser that is open now, and who may watch it. |
| [User](user.md) | A person who can sign in. Global to the install. |
| [API key](api-key.md) | A credential for a program, carrying scopes. |

The shape in one sentence: a **project** holds **apps**; a commit pushed to an
app creates a **deployment**, which produces one **build** made of
**artifacts**; invoking an app creates an **execution** against the build that
app is active on.

An app is **executed, not hosted** — there is no resident process between calls,
and the meter is the compute an execution consumes. [Servers](server.md) invert
that and are metered by resident time instead; they are not built.

Authorization is separate from all of it. **Users** and **API keys** belong to
the install, not to a project — see [api-key.md](api-key.md). Accounts come from
`POST /v1/auth/register` and nothing else.
