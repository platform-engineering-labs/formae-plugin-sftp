# SFTP Plugin for formae

A formae plugin for managing files on SFTP servers. This plugin was created as part of the [Plugin SDK Tutorial](https://docs.formae.io/plugin-sdk/tutorial/01-scaffold/).

## Installation

```bash
make install
```

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

## Development

### Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)
- SFTP server for testing

### Building

```bash
make build      # Build plugin binary
make test       # Run unit tests
make lint       # Run linter
make install    # Build + install locally
```

### Local Testing

```bash
# Start test SFTP server
docker run -d --name sftp-test -p 2222:22 atmoz/sftp testuser:testpass:::upload

# Install plugin locally
make install

# Start formae agent
SFTP_USERNAME=testuser SFTP_PASSWORD=testpass formae agent start

# Apply example resources
formae apply --mode reconcile examples/basic/main.pkl

# Clean up
docker rm -f sftp-test
```

### Credentials

The plugin reads SFTP credentials from environment variables:

| Variable | Description |
|----------|-------------|
| `SFTP_USERNAME` | SFTP username |
| `SFTP_PASSWORD` | SFTP password |

Set these environment variables before starting the formae agent.

### Conformance Testing

Run the full CRUD lifecycle + discovery tests:

```bash
make conformance-test                  # Latest formae version
make conformance-test VERSION=0.80.0   # Specific version
```

## License

This plugin is licensed under [FSL-1.1-ALv2](LICENSE).
