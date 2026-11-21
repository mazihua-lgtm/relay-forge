#!/usr/bin/env bash
# relay-forge 一键安装：生成密码、拉起服务、等到就绪。
#
#   ./install.sh              默认监听 3000
#   PORT=8080 ./install.sh    换端口
#
# 首次运行会生成 .env（含随机密码）。之后重复运行会复用已有 .env，
# 不会改数据库密码——已经初始化过的库，改密码会让服务连不上。

set -euo pipefail

cd "$(dirname "$0")"

say() { printf '%s\n' "$*"; }
die() { printf '错误：%s\n' "$*" >&2; exit 1; }

command -v docker >/dev/null 2>&1 || die "没有找到 docker，请先安装 Docker：https://docs.docker.com/engine/install/"
docker compose version >/dev/null 2>&1 || die "没有找到 docker compose 插件，请升级 Docker 到 20.10 以上"

rand() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
  else
    LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c 48
  fi
}

if [ -f .env ]; then
  say "复用已有的 .env"
  # shellcheck disable=SC1091
  set -a; . ./.env; set +a
else
  say "生成 .env（随机密码）"
  umask 077
  cat >.env <<EOF
# relay-forge 本地配置，由 install.sh 生成。不要提交到仓库。
POSTGRES_PASSWORD=$(rand)
REDIS_PASSWORD=$(rand)
PORT=${PORT:-3000}
EOF
  umask 022
  # shellcheck disable=SC1091
  set -a; . ./.env; set +a
  say "已写入 .env"
fi

mkdir -p data logs

say "拉起服务（首次会拉取镜像，可能需要几分钟）"
if ! docker compose pull --quiet relay-forge 2>/dev/null; then
  image="${RELAY_FORGE_IMAGE:-ghcr.io/mazihua-lgtm/relay-forge:latest}"
  say ""
  say "拉取镜像失败。如果这个镜像还没发布过，先在本机构建："
  say "  docker build -t ${image} ."
  say "然后用同样的命令再跑一次 ./install.sh"
  say ""
  die "镜像 ${image} 不可用"
fi
docker compose up -d

port="${PORT:-3000}"
url="http://localhost:${port}"
say "等待服务就绪：${url}"

deadline=$(( $(date +%s) + 300 ))
until curl -fsS --max-time 3 "${url}/api/status" 2>/dev/null | grep -q '"success"[[:space:]]*:[[:space:]]*true'; do
  if [ "$(date +%s)" -ge "$deadline" ]; then
    say ""
    die "等了 5 分钟还没就绪。查看日志：docker compose logs -f relay-forge"
  fi
  if ! docker compose ps --status running --services 2>/dev/null | grep -qx relay-forge; then
    say ""
    die "容器没有在运行。查看日志：docker compose logs --tail=100 relay-forge"
  fi
  sleep 3
done

cat <<EOF

就绪。

  管理后台   ${url}
  下一步     打开上面的地址，按向导创建管理员账号
  查看日志   docker compose logs -f relay-forge
  停止服务   docker compose down
  升级       docker compose pull && docker compose up -d

密码在 .env 里，别提交、别外传。对外部署还要配 HTTPS 和域名。
EOF
