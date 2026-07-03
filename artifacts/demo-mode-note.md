# Demo Auth Mode Note

Problem: demo login is currently always enabled and hard-coded to `user_id=1`, `actor_id=1`, `world_id=1`.

Production risk: if the server is exposed with the current login handler, anyone can mint a demo token for actor 1 in world 1.

Proposed fix:

- Add an explicit `-demo-auth` flag, defaulting to `true` for local development.
- Support an env override such as `ISLANDS_DEMO_AUTH=false` for deployments.
- When demo auth is disabled, `POST /api/v1/auth/login` should reject demo credentials until a real auth provider is configured.
- Keep protected endpoints unchanged: they should continue to accept only valid signed tokens.
- Add tests for demo login enabled, demo login disabled, and protected endpoint auth behavior.
