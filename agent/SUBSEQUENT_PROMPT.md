These are the actions from your last response, each annotated with how it went:
- `attempted`: whether it was started.
- `succeeded`: whether it completed.
- `reason`: why it failed or was refused; present only when something went wrong.
- `result`: data the action returned, if any.

A failed action stops the batch, so the actions after it stay
`attempted: false`.

{performed_actions}

The page state now:

{page_state}

Respond with the next structured output.
