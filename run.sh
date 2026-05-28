#!/bin/bash

# Go + React 全栈项目运行脚本

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 后端配置
BACKEND_DIR="${BACKEND_DIR:-$ROOT_DIR/backend}"
APP_PORT="${APP_PORT:-8080}"
APP_ENV="${APP_ENV:-dev}"
BINARY_NAME="${BINARY_NAME:-server}"
BINARY_PATH="${BINARY_PATH:-$BACKEND_DIR/bin/$BINARY_NAME}"
MAIN_PKG="${MAIN_PKG:-./cmd/server}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-http://127.0.0.1:${APP_PORT}/healthz}"
LOG_DIR="${LOG_DIR:-$BACKEND_DIR/logs}"

# 前端配置
FRONTEND_DIR="${FRONTEND_DIR:-$ROOT_DIR/frontend}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

# Docker 配置
PROJECT_NAME="$(basename "$ROOT_DIR")"
DOCKER_REGISTRY="${DOCKER_REGISTRY:-your-registry.example.com/library}"
DOCKER_IMAGE="${DOCKER_IMAGE:-${DOCKER_REGISTRY}/${PROJECT_NAME}:v1.0}"
DOCKER_CONTAINER="${DOCKER_CONTAINER:-$PROJECT_NAME}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

graceful_kill() {
    for pid in "$@"; do kill "$pid" 2>/dev/null; done
    for _ in $(seq 1 20); do
        sleep 0.5
        local all_dead=true
        for pid in "$@"; do kill -0 "$pid" 2>/dev/null && all_dead=false; done
        $all_dead && return 0
    done
    for pid in "$@"; do kill -9 "$pid" 2>/dev/null; done
}

port_free() { ! lsof -ti:"$1" >/dev/null 2>&1; }

kill_port() {
    local port=$1
    local pids
    pids=$(lsof -ti:"$port" 2>/dev/null)
    [ -z "$pids" ] && return 0
    graceful_kill $pids
    for _ in $(seq 1 10); do
        port_free "$port" && return 0
        sleep 0.5
    done
    return 1
}

wait_for_port() {
    local port=$1 retries="${2:-30}"
    for _ in $(seq 1 "$retries"); do
        sleep 0.5
        lsof -ti:"$port" >/dev/null 2>&1 && return 0
    done
    return 1
}

wait_for_http() {
    local url=$1 retries="${2:-20}"
    for _ in $(seq 1 "$retries"); do
        sleep 0.5
        curl -fsS "$url" >/dev/null 2>&1 && return 0
    done
    return 1
}

# ─── 后端 ────────────────────────────────────────────────────────────────────

build_binary() {
    log_info "编译 Go 二进制..."
    mkdir -p "$BACKEND_DIR/bin"
    cd "$BACKEND_DIR" || exit 1
    if go build -o "$BINARY_PATH" "$MAIN_PKG"; then
        log_info "编译完成：$BINARY_PATH"
    else
        log_error "编译失败"
        return 1
    fi
}

start_backend() {
    log_info "启动后端服务..."

    if ! port_free "$APP_PORT"; then
        log_error "后端端口 $APP_PORT 已被占用"
        return 1
    fi

    build_binary || return 1
    mkdir -p "$LOG_DIR"

    nohup bash -lc "
        cd \"$BACKEND_DIR\" || exit 1
        APP_ENV=$APP_ENV exec \"$BINARY_PATH\"
    " > "$LOG_DIR/backend.log" 2>&1 &

    local launcher_pid=$!
    if ! kill -0 "$launcher_pid" 2>/dev/null; then
        log_error "后端启动失败，查看日志：$LOG_DIR/backend.log"
        return 1
    fi

    if wait_for_port "$APP_PORT" 30 && wait_for_http "$HEALTHCHECK_URL" 20; then
        local pid
        pid=$(lsof -ti:"$APP_PORT" 2>/dev/null | head -1)
        log_info "后端已启动 (PID: $pid, 端口: $APP_PORT)"
        log_info "后端日志：$LOG_DIR/backend.log"
    else
        log_error "后端启动失败，查看日志：$LOG_DIR/backend.log"
        return 1
    fi
}

stop_backend() {
    if port_free "$APP_PORT"; then
        log_info "后端未运行"
        return 0
    fi
    log_info "停止后端 (端口 $APP_PORT)..."
    kill_port "$APP_PORT"
    log_info "后端已停止"
}

# ─── 前端 ────────────────────────────────────────────────────────────────────

start_frontend() {
    if [ ! -f "$FRONTEND_DIR/package.json" ]; then
        log_warn "前端未初始化 (缺少 package.json)，跳过"
        return 0
    fi

    log_info "启动前端服务..."

    if ! port_free "$FRONTEND_PORT"; then
        log_error "前端端口 $FRONTEND_PORT 已被占用"
        return 1
    fi

    mkdir -p "$FRONTEND_DIR/logs"

    nohup bash -lc "
        cd \"$FRONTEND_DIR\" || exit 1
        command -v pnpm >/dev/null 2>&1 || { echo 'pnpm 未安装'; exit 1; }
        exec pnpm dev --host 0.0.0.0 --port \"$FRONTEND_PORT\"
    " > "$FRONTEND_DIR/logs/frontend.log" 2>&1 &

    local launcher_pid=$!
    if ! kill -0 "$launcher_pid" 2>/dev/null; then
        log_error "前端启动失败，查看日志：$FRONTEND_DIR/logs/frontend.log"
        return 1
    fi

    if wait_for_port "$FRONTEND_PORT" 30 && wait_for_http "http://127.0.0.1:$FRONTEND_PORT" 20; then
        local pid
        pid=$(lsof -ti:"$FRONTEND_PORT" 2>/dev/null | head -1)
        log_info "前端已启动 (PID: $pid, 端口: $FRONTEND_PORT)"
        log_info "前端日志：$FRONTEND_DIR/logs/frontend.log"
    else
        log_error "前端启动失败，查看日志：$FRONTEND_DIR/logs/frontend.log"
        return 1
    fi
}

stop_frontend() {
    if port_free "$FRONTEND_PORT"; then
        log_info "前端未运行"
        return 0
    fi
    log_info "停止前端 (端口 $FRONTEND_PORT)..."
    kill_port "$FRONTEND_PORT"
    log_info "前端已停止"
}

# ─── 安装依赖 ─────────────────────────────────────────────────────────────────

install_backend() {
    log_info "检查 Go 环境..."
    go version || { log_error "Go 未安装"; return 1; }
    log_info "下载后端依赖..."
    cd "$BACKEND_DIR" && go mod download && log_info "后端依赖安装完成"
}

install_frontend() {
    if [ ! -f "$FRONTEND_DIR/package.json" ]; then
        log_warn "前端未初始化，跳过"
        return 0
    fi
    log_info "安装前端依赖..."
    (
        cd "$FRONTEND_DIR" || exit 1
        command -v pnpm >/dev/null 2>&1 || { log_error "pnpm 未安装"; exit 1; }
        pnpm install || exit 1
    ) && log_info "前端依赖安装完成" || { log_error "前端依赖安装失败"; return 1; }
}

# ─── 工具命令 ─────────────────────────────────────────────────────────────────

run_tests() {
    local target="${1:-backend}"
    case "$target" in
        backend)
            log_info "运行后端测试..."
            cd "$BACKEND_DIR" || exit 1
            go test ./... -v -count=1
            ;;
        frontend)
            log_info "运行前端测试..."
            cd "$FRONTEND_DIR" || exit 1
            pnpm test -- --passWithNoTests
            ;;
        all)
            run_tests backend || return 1
            run_tests frontend
            ;;
        *) log_error "未知目标：test $target"; return 1 ;;
    esac
}

run_lint() {
    local target="${1:-backend}"
    case "$target" in
        backend)
            log_info "运行后端代码检查..."
            cd "$BACKEND_DIR" || exit 1
            if command -v golangci-lint >/dev/null 2>&1; then
                golangci-lint run ./...
            else
                log_warn "golangci-lint 未安装，使用 go vet"
                go vet ./...
            fi
            ;;
        frontend)
            log_info "运行前端代码检查..."
            cd "$FRONTEND_DIR" || exit 1
            pnpm lint
            ;;
        all)
            run_lint backend || return 1
            run_lint frontend
            ;;
        *) log_error "未知目标：lint $target"; return 1 ;;
    esac
}

run_format() {
    local target="${1:-backend}"
    case "$target" in
        backend)
            log_info "格式化后端代码..."
            cd "$BACKEND_DIR" || exit 1
            gofmt -w $(find . -name "*.go" -not -path "./pkg/aikit/*")
            ;;
        frontend)
            log_info "格式化前端代码..."
            cd "$FRONTEND_DIR" || exit 1
            pnpm format
            ;;
        all)
            run_format backend || return 1
            run_format frontend
            ;;
        *) log_error "未知目标：format $target"; return 1 ;;
    esac
}

install_pre_commit() {
    log_info "安装 pre-commit Git hooks..."
    if ! command -v pre-commit >/dev/null 2>&1; then
        log_error "pre-commit 未安装，请执行：pip install pre-commit"
        return 1
    fi
    pre-commit install --hook-type pre-commit
    pre-commit install --hook-type pre-push
    log_info "pre-commit hooks 安装完成"
}

run_migrate() {
    log_info "执行数据库迁移..."
    cd "$BACKEND_DIR" || exit 1
    APP_ENV=$APP_ENV go run ./cmd/migrate
}

gen_swagger() {
    log_info "生成 Swagger 文档..."
    cd "$BACKEND_DIR" || exit 1
    if command -v swag >/dev/null 2>&1; then
        swag init -g cmd/server/main.go -o docs
        log_info "Swagger 文档已生成到 backend/docs/"
    else
        log_error "swag 未安装，请执行：go install github.com/swaggo/swag/cmd/swag@latest"
        return 1
    fi
}

# ─── 状态 & Docker ─────────────────────────────────────────────────────────────

status() {
    echo "=== 服务状态 ==="
    if ! port_free "$APP_PORT"; then
        local pid; pid=$(lsof -ti:"$APP_PORT" 2>/dev/null | head -1)
        log_info "后端：运行中 (端口 $APP_PORT, PID: $pid)"
    else
        log_warn "后端：未运行"
    fi
    if ! port_free "$FRONTEND_PORT"; then
        local pid; pid=$(lsof -ti:"$FRONTEND_PORT" 2>/dev/null | head -1)
        log_info "前端：运行中 (端口 $FRONTEND_PORT, PID: $pid)"
    else
        log_warn "前端：未运行"
    fi
}

docker_run() {
    docker run -d \
        --name "$DOCKER_CONTAINER" \
        --restart unless-stopped \
        --network host \
        -v /data:/data \
        -e APP_ENV=prod \
        "$DOCKER_IMAGE"
}

docker_debug() {
    docker run --rm -ti \
        --name "${DOCKER_CONTAINER}-debug" \
        --network host \
        -v /data:/data \
        -e APP_ENV=prod \
        --entrypoint "" \
        "$DOCKER_IMAGE" /bin/sh
}

docker_stop() {
    for c in "$DOCKER_CONTAINER" "${DOCKER_CONTAINER}-debug"; do
        if docker ps -a --format '{{.Names}}' | grep -qx "$c"; then
            log_info "停止容器 $c..."
            docker stop "$c" >/dev/null 2>&1
            docker rm "$c" >/dev/null 2>&1
        fi
    done
}

docker_status() {
    for c in "$DOCKER_CONTAINER" "${DOCKER_CONTAINER}-debug"; do
        if docker ps --format '{{.Names}}' | grep -qx "$c"; then
            log_info "Docker 容器 $c：运行中"
        elif docker ps -a --format '{{.Names}}' | grep -qx "$c"; then
            log_warn "Docker 容器 $c：已停止"
        fi
    done
}

# ─── 主入口 ───────────────────────────────────────────────────────────────────

case "${1:-}" in
    start)
        start_backend || exit 1
        start_frontend
        status
        ;;
    stop)
        stop_backend
        stop_frontend
        ;;
    restart)
        case "${2:-all}" in
            backend)  stop_backend;  sleep 1; start_backend || exit 1 ;;
            frontend) stop_frontend; sleep 1; start_frontend ;;
            all)      stop_backend; stop_frontend; sleep 1; start_backend || exit 1; start_frontend ;;
            *) log_error "未知目标：restart $2"; exit 1 ;;
        esac
        ;;
    backend)
        case "${2:-start}" in
            start) start_backend || exit 1 ;;
            stop)  stop_backend ;;
            *) log_error "未知命令：backend $2"; exit 1 ;;
        esac
        ;;
    frontend)
        case "${2:-start}" in
            start) start_frontend ;;
            stop)  stop_frontend ;;
            *) log_error "未知命令：frontend $2"; exit 1 ;;
        esac
        ;;
    install)
        case "${2:-all}" in
            backend)  install_backend ;;
            frontend) install_frontend ;;
            all)      install_backend && install_frontend ;;
            *) log_error "未知目标：install $2"; exit 1 ;;
        esac
        ;;
    build)
        build_binary || exit 1
        ;;
    test)
        run_tests "${2:-backend}"
        ;;
    lint)
        run_lint "${2:-backend}"
        ;;
    format)
        run_format "${2:-backend}"
        ;;
    migrate)
        run_migrate
        ;;
    swagger)
        gen_swagger
        ;;
    pre-commit-install)
        install_pre_commit
        ;;
    docker-run)
        docker_stop 2>/dev/null
        docker_run && log_info "Docker 容器已启动：$DOCKER_CONTAINER" || { log_error "启动失败"; exit 1; }
        ;;
    docker-debug)
        docker_debug
        ;;
    docker-stop)
        docker_stop
        ;;
    docker-build)
        log_info "构建 Docker 镜像：$DOCKER_IMAGE"
        docker build -t "$DOCKER_IMAGE" "$BACKEND_DIR" && \
            log_info "镜像构建完成：$DOCKER_IMAGE" || \
            { log_error "镜像构建失败"; exit 1; }
        ;;
    status|"")
        status
        docker_status
        ;;
    help)
        echo "用法：$0 {start|stop|restart|install|backend|frontend|build|migrate|test|lint|format|swagger|pre-commit-install|docker-*|status|help}"
        echo ""
        echo "命令:"
        echo "  start                  启动后端 + 前端"
        echo "  stop                   停止后端 + 前端"
        echo "  restart [backend|frontend|all]  重启服务"
        echo "  install [backend|frontend|all]  安装依赖（默认 all）"
        echo "  backend  start|stop    仅操作后端"
        echo "  frontend start|stop    仅操作前端"
        echo "  build                  编译后端二进制"
        echo "  test   [backend|frontend|all]   运行测试（默认 backend）"
        echo "  lint   [backend|frontend|all]   代码检查（默认 backend）"
        echo "  format [backend|frontend|all]   代码格式化（默认 backend）"
        echo "  migrate                执行数据库迁移（golang-migrate）"
        echo "  swagger                生成 Swagger 文档"
        echo "  pre-commit-install     安装 Git 提交前检查 hooks"
        echo "  docker-build           构建 Docker 镜像"
        echo "  docker-run             Docker 后台运行"
        echo "  docker-debug           Docker 调试模式"
        echo "  docker-stop            停止并删除 Docker 容器"
        echo "  status                 查看服务状态"
        echo ""
        echo "可配置环境变量:"
        echo "  APP_PORT        后端端口，默认 8080"
        echo "  APP_ENV         运行环境（dev/prod），默认 dev"
        echo "  FRONTEND_PORT   前端端口，默认 5173"
        echo "  BINARY_NAME     二进制名称，默认 server"
        echo "  MAIN_PKG        main 包路径，默认 ./cmd/server（相对 backend/）"
        ;;
    *)
        log_error "未知命令：$1"
        echo "运行 '$0 help' 查看帮助"
        exit 1
        ;;
esac

exit 0
