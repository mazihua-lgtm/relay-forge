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

# 老版本生成的 .env 没有这些键，缺什么补什么，避免升级时启动失败。
ensure_env_key() {
  key="$1"
  if ! grep -q "^${key}=" .env 2>/dev/null; then
    printf '%s=%s\n' "$key" "$(rand)" >>.env
    say "已在 .env 中补充 ${key}"
  fi
}

requested_port="${PORT:-}"

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
SESSION_SECRET=$(rand)
PORT=${PORT:-3000}
EOF
  umask 022
  say "已写入 .env"
fi

ensure_env_key POSTGRES_PASSWORD
ensure_env_key REDIS_PASSWORD
ensure_env_key SESSION_SECRET
chmod 600 .env 2>/dev/null || true

# shellcheck disable=SC1091
set -a; . ./.env; set +a

# 命令行显式给的端口优先，并写回 .env，否则下次会被旧值静默覆盖。
if [ -n "$requested_port" ] && [ "${PORT:-}" != "$requested_port" ]; then
  if grep -q '^PORT=' .env; then
    sed -i.bak "s|^PORT=.*|PORT=${requested_port}|" .env && rm -f .env.bak
  else
    printf 'PORT=%s\n' "$requested_port" >>.env
  fi
  PORT="$requested_port"
  say "端口改为 ${PORT}（已写回 .env）"
fi
export PORT

mkdir -p data logs

say "拉起服务（首次会拉取镜像，可能需要几分钟）"
image="${RELAY_FORGE_IMAGE:-ghcr.io/mazihua-lgtm/relay-forge:latest}"
if ! docker compose pull --quiet relay-forge 2>/dev/null; then
  # A locally built image is a legitimate source: air-gapped installs and the
  # first run before any release exists both land here.
  if docker image inspect "$image" >/dev/null 2>&1; then
    say "拉取失败，但本地已有 ${image}，直接用本地镜像启动"
  else
    say ""
    say "拉取镜像失败，本地也没有这个镜像。先在本机构建："
    say "  docker build -t ${image} ."
    say "然后用同样的命令再跑一次 ./install.sh"
    say ""
    die "镜像 ${image} 不可用"
  fi
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
