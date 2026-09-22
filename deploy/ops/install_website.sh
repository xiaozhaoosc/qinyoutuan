#!/bin/bash
# 部署官网 rouroujuxuan.top：生成自签回源证书 + nginx vhost（80/443 都 serve）+ 静态页面。
#
# 域名 rouroujuxuan.top 由 Cloudflare 代理（走 CF 边缘）。访客与 CF 之间用 CF 自身证书；
# 源站只需给 CF 回源提供任一证书。用自签即可（CF 后台该域名 SSL 模式设为 Full / Flexible）。
# 兼容 CF Flexible（80 回源）：80 端口也直接 serve，不做 301，避免 https 自循环。
#
# 用法：把 website_index.html 放到脚本同目录，sudo bash install_website.sh
set -e
DOMAIN="rouroujuxuan.top"
WEB=/var/www/rouroujuxuan
SSL=/etc/nginx/ssl/rouroujuxuan.top
SRC="$(cd "$(dirname "$0")" && pwd)"

sudo mkdir -p "$WEB"
sudo install -m 644 "$SRC/website_index.html" "$WEB/index.html"

sudo mkdir -p "$SSL"
if [ ! -f "$SSL/fullchain.pem" ]; then
  sudo openssl req -x509 -nodes -newkey rsa:2048 -days 3650 -keyout "$SSL/privkey.pem" \
    -out "$SSL/fullchain.pem" -subj "/CN=$DOMAIN" \
    -addext "subjectAltName=DNS:$DOMAIN,DNS:www.$DOMAIN" >/dev/null 2>&1
  sudo chmod 600 "$SSL/privkey.pem"
fi

cat <<NGINX | sudo tee /etc/nginx/sites-available/rouroujuxuan > /dev/null
server {
  listen 80;
  server_name rouroujuxuan.top www.rouroujuxuan.top;
  root $WEB;
  index index.html;
  location / { try_files \$uri \$uri/ =404; }
}
server {
  listen 443 ssl;
  http2 on;
  server_name rouroujuxuan.top www.rouroujuxuan.top;
  root $WEB;
  index index.html;
  ssl_certificate $SSL/fullchain.pem;
  ssl_certificate_key $SSL/privkey.pem;
  ssl_protocols TLSv1.2 TLSv1.3;
  location / { try_files \$uri \$uri/ =404; }
}
NGINX
sudo ln -sf /etc/nginx/sites-available/rouroujuxuan /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

echo "=== 本机验证 ==="
curl -sk -o /dev/null -w 'https443=%{http_code}\n' https://127.0.0.1/ -H "Host: rouroujuxuan.top"
curl -sk -o /dev/null -w 'http80=%{http_code}\n' http://127.0.0.1/ -H "Host: rouroujuxuan.top"