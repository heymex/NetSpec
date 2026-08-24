# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.0.0   | :white_check_mark: |
| 1.0.x   | :x:                |


## Security Best Practices

### TLS/HTTPS Configuration

**CRITICAL**: NetSpec transmits sensitive data including:
- Administrator passwords during login
- Session cookies for authentication
- API bearer tokens
- SNMP community strings
- Device configurations and credentials

**Always use TLS in production environments** to protect this data from interception and tampering.

#### Configuring TLS

Set the following environment variables to enable HTTPS:

```bash
TLS_CERT_PATH=/path/to/certificate.pem
TLS_KEY_PATH=/path/to/private-key.pem
```

#### Obtaining TLS Certificates

**Option 1: Let's Encrypt (Recommended for production)**
```bash
# Install certbot
sudo apt-get install certbot

# Obtain certificate
sudo certbot certonly --standalone -d netspec.example.com

# Configure NetSpec
TLS_CERT_PATH=/etc/letsencrypt/live/netspec.example.com/fullchain.pem
TLS_KEY_PATH=/etc/letsencrypt/live/netspec.example.com/privkey.pem
```

**Option 2: Self-signed certificate (Testing/internal use only)**
```bash
# Generate self-signed certificate
mkdir -p /opt/netspec/tls
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout /opt/netspec/tls/server.key \
  -out /opt/netspec/tls/server.crt \
  -days 365 -subj "/CN=netspec.local"

# Configure NetSpec
TLS_CERT_PATH=/opt/netspec/tls/server.crt
TLS_KEY_PATH=/opt/netspec/tls/server.key
```

**Note**: Self-signed certificates will trigger browser warnings. For production, use certificates from a trusted Certificate Authority.

#### Docker Compose Configuration

When using Docker Compose, mount your certificate files as volumes:

```yaml
services:
  netspec-netspec:
    volumes:
      - /etc/letsencrypt:/etc/letsencrypt:ro
      - /opt/netspec/config:/config
      - /opt/netspec/data:/data
    environment:
      - TLS_CERT_PATH=/etc/letsencrypt/live/netspec.example.com/fullchain.pem
      - TLS_KEY_PATH=/etc/letsencrypt/live/netspec.example.com/privkey.pem
```

### Authentication

Enable authentication by setting a password hash:

```bash
# Generate password hash
docker run --rm ghcr.io/heymex/netspec:latest hash-password

# Set in environment
NETSPEC_ADMIN_PASSWORD_HASH=<generated-hash>
```

### Session Security

Session cookies are configured with:
- `HttpOnly`: Prevents JavaScript access
- `Secure`: Requires HTTPS (when TLS is enabled)
- `SameSite=Lax`: Provides CSRF protection

### Network Security

- Deploy behind a reverse proxy (nginx, Traefik, Caddy) for additional security layers
- Use firewall rules to restrict access to trusted networks
- Consider VPN or zero-trust network access for remote administration

## Reporting a Vulnerability

Open an issue in the repo and I will contact you directly via the method you list. This is a very, very side/part time thing for me, so expect the app to be pulled if there's a critical severity issue reported.
