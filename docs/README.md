# Concepts

Nine objects. Read them in this order — each one only makes sense on top of
the last.

| Concept | One line |
| --- | --- |
| [Project](project.md) | A namespace that owns apps and everything under them. |
| [App](app.md) | A named thing you deploy to. Must exist before a deploy. |
| [Deployment](deployment.md) | One upload of source, and the builds made from it. |
| [Build](build.md) | One attempt at turning that source into runnable artifacts. |
| [Artifact](artifact.md) | An immutable blob a build produced or a deploy uploaded. |
| [Execution](execution.md) | One invocation of a built handler. The billable unit. |
| [Browser profile](browser-profile.md) | Who a browser appears to be, and what it remembers. |
| [Browser session](browser-session.md) | A browser that is open now, and who may watch it. |
| [User](user.md) | A person who can sign in. Global to the install. |
| [API key](api-key.md) | A credential for a program, carrying scopes. |

The shape in one sentence: a **project** holds **apps**; deploying source to an
app creates a **deployment**, which produces **builds** made of **artifacts**;
invoking a ready build creates an **execution**.

An app is **executed, not hosted** — there is no resident process between calls,
and the meter is the compute an execution consumes. [Servers](server.md) invert
that and are metered by resident time instead; they are not built.

Authorization is separate from all of it. **Users** and **API keys** belong to
the install, not to a project — see [api-key.md](api-key.md). Accounts come from
`POST /v1/auth/register` and nothing else.
