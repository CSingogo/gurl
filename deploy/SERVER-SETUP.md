# One-time VPS setup for gurl-server

Run on the VPS (Ubuntu 24.04) as the `somo` user. After this, every push to `main`
deploys with no manual steps.

```sh
# 1. An unprivileged account that only runs the service (no login shell, no home).
sudo useradd --system --no-create-home --shell /usr/sbin/nologin gurl

# 2. Where the binary lives. `somo` owns the directory so CI can drop a new binary
#    in; `gurl` can read and run it but cannot modify it.
sudo install -d -o somo -g gurl -m 755 /opt/gurl

# 3. The service.
sudo install -m 644 gurl.service /etc/systemd/system/gurl.service   # from deploy/
sudo systemctl daemon-reload
sudo systemctl enable gurl            # start at boot (it starts for real after the first deploy)

# 4. Let `somo` restart that one service without a password, and nothing else.
echo 'somo ALL=(root) NOPASSWD: /usr/bin/systemctl restart gurl' | sudo tee /etc/sudoers.d/gurl-deploy
sudo chmod 440 /etc/sudoers.d/gurl-deploy
sudo visudo -cf /etc/sudoers.d/gurl-deploy      # must print "parsed OK"

# 5. Caddy: add the block from Caddyfile.snippet, then
sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl reload caddy
```

DNS (Cloudflare dashboard): `A` record, name `gurl`, value `31.97.114.157`, proxied (orange cloud).

Useful afterwards:

```sh
systemctl status gurl
journalctl -u gurl -f          # live request log
systemd-analyze security gurl  # how well sandboxed the service is
```
