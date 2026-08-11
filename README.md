# SFTP Plugin for formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-sftp/actions/workflows/ci.yml/badge.svg)](https://github.com/platform-engineering-labs/formae-plugin-sftp/actions/workflows/ci.yml)

A formae plugin for managing files on SFTP servers. This plugin was created as part of the [Plugin SDK Tutorial](https://docs.formae.io/plugin-sdk/tutorial/01-scaffold/).

## Supported Resources

| Resource Type | Description |
|---------------|-------------|
| `SFTP::Files::File` | Manages files on an SFTP server |

## Configuration

Configure a target in your forma file:

```pkl
import "@sftp/sftp.pkl"

new formae.Target {
  label = "sftp-server"
  config = new sftp.Config {
    url = "sftp://hostname:22"
  }
}
```

### Credentials

Credentials can come from the target config, which is the recommended form
because both fields accept a resolvable and can therefore be sourced from a
formae-managed secret. The agent resolves them live before every call, so
onboarding a server or rotating a password needs no agent restart:

```pkl
config = new sftp.Config {
  url = "sftp://hostname:22"
  username = "formae"
  password = sftpPassword.res.secretValue
}
```

A declared credential is used as given: one that resolves to an empty value is
an error rather than a silent fall back, which would otherwise log in as
whoever the environment names.

Each credential falls back independently, so a literal username can sit beside
a password sourced from a secret. Whichever is not declared comes from the
environment:

| Variable | Description |
|----------|-------------|
| `SFTP_USERNAME` | SFTP username |
| `SFTP_PASSWORD` | SFTP password |

Set those environment variables before starting the formae agent if you are
not declaring the matching credential in the target config.

## Examples

See the [examples/](examples/) directory for usage examples.

```pkl
import "@sftp/sftp.pkl"

new sftp.File {
  label = "hello"
  path = "/upload/hello.txt"
  content = "Hello from formae!"
  permissions = "0644"
}
```

```bash
# Apply resources
formae apply --mode reconcile examples/basic/main.pkl
```

## License

This plugin is licensed under [FSL-1.1-ALv2](LICENSE).
