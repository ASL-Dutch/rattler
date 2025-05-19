# Tax Document Service with Caddy

This service provides access to BE and NL tax documents using Caddy as a web server.

## Configuration

The service uses a Caddyfile to configure file serving with the following features:

- Separate domains for BE and NL tax documents
- Smart path handling for dated files (YYYYMM_filename.pdf)
- Fallback to BE directory for files without date prefixes
- Security features (Basic auth, security headers, logging)
- Directory listing disabled

## Getting Started

1. Install Caddy from [caddyserver.com](https://caddyserver.com/docs/install)

2. Create log directory:
   ```
   mkdir -p /var/log/caddy
   ```

3. Update password in Caddyfile:
   ```
   caddy hash-password
   ```
   Replace the hash in the Caddyfile with your generated hash.

4. Run Caddy:
   ```
   caddy run
   ```

## Access

- BE tax documents: http://be.tax.documents.local/YYYYMM_filename.pdf
  - Example: http://be.tax.documents.local/202505_test_tax_bill_01.pdf
  - For non-dated files: http://be.tax.documents.local/test_tax_bill_01.pdf

- NL tax documents: http://nl.tax.documents.local/YYYYMM_filename.pdf
  - Example: http://nl.tax.documents.local/202505_test_tax_bill_01.pdf
  - Non-dated files are searched in BE directory

## Directory Structure

The tax documents are stored in the following structure:
```
out/backup/pdf/
├── be/
│   └── YYYY/
│       └── MM/
│           └── YYYYMM_filename.pdf
└── nl/
    └── YYYY/
        └── MM/
            └── YYYYMM_filename.pdf
```

## Security Considerations

- Basic authentication is enabled
- Security headers are configured
- Directory listing is disabled
- Server information is removed from headers
