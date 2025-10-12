# Configuration Files

This directory contains configuration files for the EMQX-Go project.

## Files

| File | Description |
|------|-------------|
| `config.json` | Example JSON configuration file |
| `package.json` | Node.js dependencies for test scripts |
| `package-lock.json` | Locked versions of Node.js dependencies |

## Configuration Format

EMQX-Go supports both YAML and JSON configuration formats.

### Example: config.json

```json
{
  "broker": {
    "node_id": "emqx-go-node",
    "mqtt_port": ":1883",
    "grpc_port": ":8081",
    "metrics_port": ":8082",
    "auth": {
      "enabled": true,
      "users": [
        {
          "username": "admin",
          "password": "admin_password",
          "algorithm": "bcrypt",
          "enabled": true
        }
      ]
    }
  }
}
```

## Using Configuration Files

Load a configuration file when starting the broker:

```bash
# Using JSON config
./emqx-go -config configs/config.json

# Using YAML config
./emqx-go -config configs/config.yaml
```

## Node.js Dependencies

The `package.json` file defines dependencies for MQTT test scripts:

```bash
# Install dependencies
cd configs
npm install

# Or from project root
npm install --prefix configs/
```

## Related Documentation

- [Configuration Guide](../docs/guides/CONFIG_GUIDE.md)
- [Deployment Guide](../docs/guides/DEPLOYMENT.md)
