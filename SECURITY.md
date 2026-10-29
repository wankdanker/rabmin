# Security policy

rabmin holds credentials for RabbitMQ clusters and can change them, so security reports are taken seriously.

## Reporting a vulnerability

Please report vulnerabilities privately through [GitHub security advisories](https://github.com/wankdanker/rabmin/security/advisories/new). Do not open a public issue.

Include the rabmin version, how to reproduce, and the impact you see. You should get a reply within a few days.

## Supported versions

Fixes go into the latest release. Before 1.0, only the most recent minor version is supported.

## Hardening checklist for deployments

- Serve rabmin over HTTPS (`RABMIN_TLS_CERT`/`RABMIN_TLS_KEY` or a reverse proxy) and set `RABMIN_SECURE_COOKIES=true`.
- Set `RABMIN_ADMIN_USER`/`RABMIN_ADMIN_PASSWORD` or `RABMIN_ALLOW_SETUP=false` before exposing a fresh instance, so nobody else can claim the first admin account.
- Back up `RABMIN_SECRET_KEY` (or `secret.key` in the data directory); stored RabbitMQ credentials cannot be decrypted without it.
- Give rabmin's RabbitMQ users only the permissions they need: a user tagged `monitoring` is enough to observe a cluster; `administrator` is needed to change users, policies and definitions.
- Restrict who can reach the RabbitMQ management, Prometheus and AMQP ports; rabmin needs them, browsers do not.
