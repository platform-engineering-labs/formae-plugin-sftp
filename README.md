# SFTP plugin for formae

[![CI](https://github.com/platform-engineering-labs/formae-plugin-sftp/actions/workflows/ci.yml/badge.svg)](https://github.com/platform-engineering-labs/formae-plugin-sftp/actions/workflows/ci.yml)

A formae plugin for managing files on SFTP servers. This plugin was created as part of the [Plugin SDK Tutorial](https://docs.formae.ai/plugin-development/tutorial/01-scaffold).

[formae](https://github.com/platform-engineering-labs/formae) · [Hub](https://hub.platform.engineering/platform.engineering/sftp)

## Install

Requires the formae CLI: see the [quick start](https://docs.formae.ai/documentation/get-started/quickstart).

```bash
formae plugin install sftp
```

Restart the formae agent afterwards so it loads the plugin.

**New project:** with the agent running, `formae project init --include sftp my-project` creates `my-project` with a `PklProject` that declares the formae and sftp schema packages, so `import "@sftp/..."` resolves, and a starter `main.pkl`. Don't run it in an existing project: it overwrites both files.

**Existing project:** add the plugin to `dependencies` in your `PklProject`, with the current version from the [hub page](https://hub.platform.engineering/platform.engineering/sftp), then run `pkl project resolve`:

```pkl
["sftp"] {
  uri = "package://hub.platform.engineering/plugins/sftp/schema/pkl/sftp/sftp@<version>"
}
```

Next: [write your first forma](https://docs.formae.ai/documentation/get-started/write-your-first-forma), then [`formae apply`](https://docs.formae.ai/documentation/reference/cli/apply) (see [apply modes](https://docs.formae.ai/documentation/concepts/apply-modes)).

With an AI coding assistant, use the [formae plugin](https://docs.formae.ai/documentation/guides/ai-coding-assistants) (formerly `formae-mcp`), which can search the hub and fetch plugin examples. The formae documentation is also available as [llms.txt](https://docs.formae.ai/llms.txt).

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
