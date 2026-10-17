# Authentication

Garage Web UI supports local passwords, a trusted authentication proxy, and OpenID Connect (OIDC). Select one with `--auth-mode` or `GARAGE_WEBUI__AUTH_MODE`. Configuration flags can also be supplied as environment variables: uppercase the flag name, replace hyphens with underscores, and prefix it with `GARAGE_WEBUI__`.

All modes require PostgreSQL, a Garage Admin API URL and token, and a browser-accessible `--public-url`. Set the public URL to the external HTTP(S) origin, without a path; use `--route-prefix` separately when hosting under a path. Use HTTPS for a production deployment and keep credentials in a secret store.

## Local passwords

Set `--auth-mode=local`, open the Web UI, and create the first administrator when prompted. Setup is available only until an administrator exists. A password must be 12–72 bytes long. Subsequent accounts and their roles can be managed by an administrator.

```sh
export GARAGE_WEBUI__AUTH_MODE=local
export GARAGE_WEBUI__PUBLIC_URL=https://garage.example.com
```

Local sign-in uses `/login` beneath any configured route prefix. Protect the first-setup page from untrusted access until the administrator account has been created.

## Trusted proxy

In proxy mode an upstream authentication proxy supplies the user identity and groups in HTTP headers. Configure the immediate proxy's IP networks and the group that grants administrator access:

```sh
export GARAGE_WEBUI__AUTH_MODE=proxy
export GARAGE_WEBUI__TRUSTED_PROXY_CIDRS=10.0.0.0/24
export GARAGE_WEBUI__TRUSTED_ADMIN_GROUP=garage-admins
export GARAGE_WEBUI__PUBLIC_URL=https://garage.example.com
```

Replace the example network with the actual proxy peer addresses. Only trusted peers may assert identity. Isolate the Web UI from direct client access and configure the proxy to remove client-supplied identity headers before setting its own; otherwise a client could impersonate a user or administrator. Avoid trusting every peer (`*`) unless ingress is isolated.

The default headers, checked in order, are:

| Identity field | Headers                                                                     |
| -------------- | --------------------------------------------------------------------------- |
| Username       | `X-Forwarded-User`, `X-Auth-Request-User`, `Remote-User`                    |
| Email          | `X-Forwarded-Email`, `X-Auth-Request-Email`, `X-Authentik-Email`            |
| Display name   | `X-Forwarded-Name`, `X-Auth-Request-Preferred-Username`, `X-Authentik-Name` |
| Groups         | `X-Forwarded-Groups`, `X-Auth-Request-Groups`                               |

Use `--trusted-username-headers`, `--trusted-email-headers`, `--trusted-display-name-headers`, and `--trusted-group-headers` to override these lists. An exact match with `--trusted-admin-group` grants the effective administrator role; otherwise the account's local role applies.

## OpenID Connect

Register an OIDC client with your identity provider, then configure the issuer, client ID, client secret, and administrator group. The provider must publish discovery metadata and include the group claim in the ID token.

```sh
export GARAGE_WEBUI__AUTH_MODE=oidc
export GARAGE_WEBUI__PUBLIC_URL=https://garage.example.com
export GARAGE_WEBUI__OIDC_ISSUER=https://identity.example.com
export GARAGE_WEBUI__OIDC_CLIENT_ID=garage-webui
export GARAGE_WEBUI__OIDC_CLIENT_SECRET='<client-secret>'
export GARAGE_WEBUI__OIDC_ADMIN_GROUP=garage-admins
```

Register the redirect URI as the public origin, followed by the route prefix, if any, and `/auth/callback`. For example, with `--route-prefix=/garage`, use `https://garage.example.com/garage/auth/callback`. The application uses the authorization-code flow with PKCE and requests `openid`, `profile`, and `email` scopes.

The group claim defaults to `groups`; change it with `--oidc-group-claim`. A string or an array of strings is supported. A group matching `--oidc-admin-group` grants the effective administrator role. OIDC group mappings can also associate external groups with local groups; mapped memberships are synchronized on verified login. The identity is keyed by issuer and subject, while `preferred_username` is used as the suggested account name when present.

## Accounts and registration

External identities are linked to local accounts. When external registration is allowed, a new identity can create a viewer account; an existing username is not automatically taken over. An unlinked OIDC identity can be placed in the pending list for administrator review, including when registration is disabled or the suggested username conflicts. Administrators can manage accounts, identity links, groups, roles, and the registration policy in the Web UI. Disabled accounts cannot authenticate.

## Recovery and sessions

With proxy or OIDC mode, enable `--local-login` to allow a local credential to sign in at `/login` beneath the route prefix. The account must already have a password and an enabled local credential. Plan and test recovery access before relying on an external provider; enabling the flag alone does not create an account.

Local and OIDC browser sessions use HTTP-only cookies scoped to the route prefix. They are marked Secure when the public URL uses HTTPS. Logout revokes the current session; administrators can revoke an account's sessions. In proxy mode, signing out of a local session does not itself sign the user out of the upstream proxy.

Up to 1,000 expired session records and 1,000 expired OIDC state records per application instance are removed at startup and approximately hourly while the application is running. Larger backlogs are cleared over subsequent runs.

## Password attempt limits

Password attempts are limited per immediate network peer, independently on each application process. Behind a reverse proxy, multiple users may share the proxy's limit; across replicas, each process maintains its own limit. Configure rate limiting at a trusted reverse proxy or ingress for deployment-wide protection and appropriate per-client limits. The application does not trust forwarded client addresses for its own limiter.
