# Claude project notes

## Available project skill

- Use `.claude/skills/add-webhook/SKILL.md` for requests to add or extend a webhook.

## Webhook rule for this repository

- Only create a new webhook for a new GVK.
- If a webhook already exists for that GVK, extend the existing implementation instead of generating a second webhook.
