# Tax Document Service with Caddy

This service provides access to BE and NL tax documents using Caddy as a web server.

## Configuration

The service uses a Caddyfile to configure file serving with **dual-root** support:

- **Dated files**: URL `/YYYYMM_filename.pdf` → served from `backup-dir/YYYY/MM/YYYYMM_filename.pdf`
- **Non-dated files**: URL `/filename.pdf` → served from the **original path** root (same as config `storage.*.tax-bill` or watch-dir), for historical flat files and (when `keep-original: true`) new files
- Separate domains for BE and NL; each site has its own date root (backup-dir) and non-date root (original path)
- Security headers, logging; directory listing disabled

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

- **Dated**: `http://be.tax.local/202505_test_tax_bill_01.pdf` → file at `backup-dir/2025/05/202505_test_tax_bill_01.pdf`
- **Non-dated**: `http://be.tax.local/old_file.pdf` → file at the **original path** root (e.g. `storage.be.tax-bill` or watch-dir)

Same for NL: dated from backup-dir, non-dated from NL original path. This supports ASL-style deployments where many historical files remain in the flat original path.

## Directory Structure

- **Date root (backup-dir)**: `backup-dir/YYYY/MM/YYYYMM_filename.pdf`
- **Non-date root (original path)**: flat files under `storage.*.tax-bill` or `watchers.pdf.*.watch-dir`

## Security Considerations

- Basic authentication is enabled
- Security headers are configured
- Directory listing is disabled
- Server information is removed from headers
