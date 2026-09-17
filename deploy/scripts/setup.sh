#!/usr/bin/env bash
# TripWeave 服务器首次初始化（Ubuntu on Oracle A1）。
# 只做一次：装 Docker、放通 80/443（Oracle Ubuntu 镜像的 iptables 默认 REJECT 非 SSH）。
# 用法：sudo bash deploy/scripts/setup.sh
set -euo pipefail

echo "==> 安装 Docker"
if ! command -v docker >/dev/null 2>&1; then
  apt-get update
  apt-get install -y ca-certificates curl gnupg
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  # 允许 ubuntu 用户免 sudo 跑 docker
  usermod -aG docker ubuntu || true
fi

echo "==> 放通 80/443（系统 iptables）"
# Oracle Ubuntu 镜像在 iptables 尾部有 REJECT 规则，端口要在它之前 ACCEPT。
for port in 80 443; do
  if ! iptables -C INPUT -p tcp --dport "$port" -j ACCEPT 2>/dev/null; then
    iptables -I INPUT 5 -p tcp --dport "$port" -j ACCEPT -m comment --comment "tripweave"
  fi
done
if command -v netfilter-persistent >/dev/null 2>&1; then
  netfilter-persistent save
else
  apt-get install -y iptables-persistent && netfilter-persistent save
fi

echo "==> 完成。接下来:"
echo "  1. git clone <你的仓库> && cd tripweave"
echo "  2. cp deploy/.env.example .env.prod 并填入机密"
echo "  3. bash deploy/scripts/deploy.sh"
