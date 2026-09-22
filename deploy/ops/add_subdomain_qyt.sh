#!/bin/bash
# 新增一级子域名 qyt.rouroujuxuan.top → 反代到 192.236.223.225:8086
# 复用当前 Origin 通配证书（*.rouroujuxuan.top 覆盖 qyt）；80 强制跳 443。
set -e
DOMAIN="qyt.rouroujuxuan.top"
UPSTREAM="http://192.236.223.225:8086"
SSL="/etc/nginx/ssl/rouroujuxuan.top"

cat <<NGINX | sudo tee /etc/nginx/sites-available/qyt-rourou > /dev/null
# qyt 反代：qyt.rouroujuxuan.top -> ${UPSTREAM}
# 80 强制跳 443
server {
  listen 80;
  server_name ${DOMAIN};
  return 301 https://\$host\$request_uri;
}

# 443（复用 Origin 通配证书）
server {
  listen 443 ssl;
  http2 on;
  server_name ${DOMAIN};
  ssl_certificate ${SSL}/fullchain.pem;
  ssl_certificate_key ${SSL}/privkey.pem;
  ssl_protocols TLSv1.2 TLSv1.3;
  client_max_body_size 20m;

  location / {
    proxy_pass ${UPSTREAM};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_set_header Upgrade \$http_upgrade;
    proxy_set_header Connection "upgrade";
  }
}
NGINX

sudo ln -sf /etc/nginx/sites-available/qyt-rourou /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

echo "=== 本机验证（走 443）==="
curl -sk -o /dev/null -w 'https443_code=%{http_code}\n' -m 15 https://127.0.0.1/ -H "Host: ${DOMAIN}"
echo "=== 本机验证（80 应 301 到 https）==="
curl -s -o /dev/null -w 'http80_code=%{http_code} loc=%{redirect_url}\n' -m 15 http://127.0.0.1/ -H "Host: ${DOMAIN}"