# Concepts

Eight objects. Read them in this order — each one only makes sense on top of
the last.

| Concept | One line |
| --- | --- |
| [Project](project.md) | A namespace that owns apps and everything under them. |
| [App](app.md) | A named thing you deploy to. Must exist before a deploy. |
| [Deployment](deployment.md) | One upload of source, and the builds made from it. |
| [Build](build.md) | One attempt at turning that source into runnable artifacts. |
| [Artifact](artifact.md) | An immutable blob a build produced or a deploy uploaded. |
| [Execution](execution.md) | One invocation of a built handler. |
| [User](user.md) | A person who can sign in. Global to the install. |
| [API key](api-key.md) | A credential for a program, carrying scopes. |

The shape in one sentence: a **project** holds **apps**; deploying source to an
app creates a **deployment**, which produces **builds** made of **artifacts**;
invoking a ready build creates an **execution**.

Authorization is separate from all of it. **Users** and **API keys** belong to
the install, not to a project — see [api-key.md](api-key.md).
