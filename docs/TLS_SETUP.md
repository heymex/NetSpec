# TLS/HTTPS Setup Guide

## Overview

NetSpec transmits sensitive information including administrator passwords, session cookies, API tokens, SNMP community strings, and device configurations. **TLS encryption is strongly recommended for all production deployments** to protect this data from interception and tampering by on-path attackers.

## Quick Start

### 1. Obtain TLS Certificates

Choose one of the following methods:

#### Option A: Let's Encrypt (Recommended for Production)

```bash
# Install certbot
sudo apt-get update
sudo apt-get install certbot

# Obtain certificate (requires port 80 to be available)
sudo certbot certonly --standalone -d netspec.example.com

# Certificates will be stored in:
# /etc/letsencrypt/live/netspec.example.com/fullchain.pem
# /etc/letsencrypt/live/netspec.example.com/privkey.pem
```

#### Option B: Self-Signed Certificate (Testing/Internal Use)

```bash
# Create directory for certificates
mkdir -p /opt/netspec/tls

# Generate self-signed certificate (valid for 365 days)
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout /opt/netspec/tls/server.key \
  -out /opt/netspec/tls/server.crt \
  -days 365 \
  -subj "/CN=netspec.local"

# Set appropriate permissions
chmod 600 /opt/netspec/tls/server.key
chmod 644 /opt/netspec/tls/server.crt
```

**Note**: Self-signed certificates will trigger browser security warnings. For production environments, use certificates from a trusted Certificate Authority like Let's Encrypt.

#### Option C: Organization PKI

If your organization has an internal PKI, request a certificate from your security team and use those files.

### 2. Configure NetSpec

#### Using Environment Variables

Add to your `.env` file:

```bash
TLS_CERT_PATH=/path/to/certificate.pem
TLS_KEY_PATH=/path/to/private-key.pem
```

#### Using Docker Compose

Update your `docker-compose.yml`:

```yaml
services:
  netspec-netspec:
    volumes:
      # Mount certificate directory (read-only for security)
      - /etc/letsencrypt:/etc/letsencrypt:ro
      # Or for self-signed:
      # - /opt/netspec/tls:/tls:ro
      - ${NETSPEC_DATA_DIR:-/opt/netspec}/config:/config
      - ${NETSPEC_DATA_DIR:-/opt/netspec}/data:/data
    environment:
      # Let's Encrypt paths
      - TLS_CERT_PATH=/etc/letsencrypt/live/netspec.example.com/fullchain.pem
      - TLS_KEY_PATH=/etc/letsencrypt/live/netspec.example.com/privkey.pem
      # Or for self-signed:
      # - TLS_CERT_PATH=/tls/server.crt
      # - TLS_KEY_PATH=/tls/server.key
```

### 3. Restart NetSpec

```bash
docker-compose down
docker-compose up -d
```

### 4. Verify HTTPS is Active

Check the logs:

```bash
docker-compose logs netspec-netspec | grep TLS
```

You should see:
```
TLS configured - server will use HTTPS
Starting API server with Web UI over TLS
```

Access your NetSpec instance at `https://your-domain:8088` (note the `https://`).

## Security Features

When TLS is enabled, NetSpec implements the following security measures:

### TLS Configuration
- **Minimum TLS Version**: TLS 1.2
- **Cipher Suites**: Strong ECDHE ciphers with AES-GCM
- **Curve Preferences**: P-521, P-384, P-256
- **Server Cipher Preference**: Enabled

### Cookie Security
- **Secure Flag**: Set to `true` (cookies only sent over HTTPS)
- **HttpOnly Flag**: Set to `true` (prevents JavaScript access)
- **SameSite**: Set to `Lax` (CSRF protection)

## Certificate Renewal

### Let's Encrypt Auto-Renewal

Let's Encrypt certificates expire after 90 days. Set up automatic renewal:

```bash
# Test renewal
sudo certbot renew --dry-run

# Set up automatic renewal (certbot usually does this automatically)
sudo systemctl enable certbot.timer
sudo systemctl start certbot.timer

# After renewal, restart NetSpec to load new certificates
sudo docker-compose restart netspec-netspec
```

### Self-Signed Certificate Renewal

Self-signed certificates must be manually renewed before expiration:

```bash
# Generate new certificate
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout /opt/netspec/tls/server.key \
  -out /opt/netspec/tls/server.crt \
  -days 365 \
  -subj "/CN=netspec.local"

# Restart NetSpec
docker-compose restart netspec-netspec
```

## Troubleshooting

### Certificate File Not Found

**Error**: `TLS certificate file not found, falling back to HTTP`

**Solution**: Verify the certificate path is correct and the file is accessible inside the container:

```bash
# Check if file exists on host
ls -la /path/to/certificate.pem

# Check if volume is mounted correctly
docker-compose exec netspec-netspec ls -la /etc/letsencrypt/live/
```

### Permission Denied

**Error**: `permission denied` when reading certificate files

**Solution**: Ensure the NetSpec container has read access:

```bash
# For Let's Encrypt (run as root or with sudo)
chmod 755 /etc/letsencrypt/live
chmod 755 /etc/letsencrypt/archive
chmod 644 /etc/letsencrypt/live/*/fullchain.pem
chmod 600 /etc/letsencrypt/live/*/privkey.pem

# For self-signed
chmod 644 /opt/netspec/tls/server.crt
chmod 600 /opt/netspec/tls/server.key
```

### Browser Certificate Warnings

**Issue**: Browser shows "Your connection is not private" or similar warning

**For Self-Signed Certificates**: This is expected. You can:
1. Click "Advanced" and proceed anyway (not recommended for production)
2. Add the certificate to your browser's trusted certificates
3. Use a certificate from a trusted CA instead

**For Let's Encrypt**: Verify:
1. The domain name matches the certificate
2. The certificate hasn't expired
3. The full certificate chain is included (use `fullchain.pem`, not `cert.pem`)

## Reverse Proxy Setup

For additional security and features, consider deploying NetSpec behind a reverse proxy:

### Nginx Example

```nginx
server {
    listen 443 ssl http2;
    server_name netspec.example.com;

    ssl_certificate /etc/letsencrypt/live/netspec.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/netspec.example.com/privkey.pem;
    
    # Strong SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    
    location / {
        proxy_pass http://localhost:8088;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name netspec.example.com;
    return 301 https://$server_name$request_uri;
}
```

### Traefik Example

```yaml
services:
  netspec-netspec:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.netspec.rule=Host(`netspec.example.com`)"
      - "traefik.http.routers.netspec.entrypoints=websecure"
      - "traefik.http.routers.netspec.tls.certresolver=letsencrypt"
      - "traefik.http.services.netspec.loadbalancer.server.port=8088"
```

## Additional Security Recommendations

1. **Enable Authentication**: Always set `NETSPEC_ADMIN_PASSWORD_HASH` in production
2. **Use Strong Passwords**: Generate with `docker run --rm ghcr.io/heymex/netspec:latest hash-password`
3. **Restrict Network Access**: Use firewall rules to limit access to trusted networks
4. **Regular Updates**: Keep NetSpec and its dependencies up to date
5. **Monitor Logs**: Review logs regularly for suspicious activity
6. **Backup Certificates**: Keep secure backups of your TLS private keys

## References

- [Let's Encrypt Documentation](https://letsencrypt.org/docs/)
- [Mozilla SSL Configuration Generator](https://ssl-config.mozilla.org/)
- [OWASP Transport Layer Protection Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Protection_Cheat_Sheet.html)
