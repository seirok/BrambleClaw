#!/usr/bin/env bash
# 示例 3: Bash Hook 脚本
# 不需要 jq 依赖

set -euo pipefail

# 读取输入
INPUT=$(cat)

# 简单的 JSON 字段提取函数
extract_string() {
    local json="$1"
    local key="$2"
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" |
        sed "s/\"$key\"[[:space:]]*:[[:space:]]*\"//;s/\"$//" |
        head -1
}

extract_number() {
    local json="$1"
    local key="$2"
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*[0-9]*\.?[0-9]*" |
        sed "s/\"$key\"[[:space:]]*:[[:space:]]*//" |
        head -1
}

# 日志输出到 stderr
log() {
    echo "[INFO] $1" >&2
}

# 解析字段
HOOK_POINT=$(extract_string "$INPUT" "hook_point")
NAME=$(extract_string "$INPUT" "name" || true)
AMOUNT=$(extract_number "$INPUT" "amount" || true)

# 设置默认值
NAME=${NAME:-"World"}
AMOUNT=${AMOUNT:-0}

log "Processing hook: $HOOK_POINT"
log "Name: $NAME, Amount: $AMOUNT"

# 业务逻辑
if [ "$AMOUNT" != "0" ] && (( $(echo "$AMOUNT > 10000" | bc -l 2>/dev/null || echo 0) )); then
    cat <<EOF
{"decision":"deny","message":"Amount $AMOUNT exceeds limit of 10000"}
EOF
    exit 0
fi

# 返回成功响应
cat <<EOF
{"decision":"allow","message":"Hello, $NAME!","extensions":{"processed":true,"from":"bash"}}
EOF
