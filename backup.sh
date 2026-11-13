#!/usr/bin/env sh

set -eu

SOURCE_DIR=$(pwd -P)
BACKUP_DIR=$(CDPATH= cd -- "$(dirname -- "$SOURCE_DIR")" && pwd -P)/ocopion-backup
BACKEND_DIR=ocopion-backend
FRONTEND_DIR=ocopion-frontend-pc

WINDOWS_RSYNC=false

case $(uname -s 2>/dev/null || true) in
  MINGW* | MSYS* | CYGWIN*)
    WINDOWS_RSYNC=true
    ;;
esac

to_rsync_path() {
  if [ "$WINDOWS_RSYNC" = false ]; then
    printf '%s\n' "$1"
    return
  fi

  windows_path=$(cygpath -am -- "$1")
  drive_letter=$(printf '%s' "$windows_path" | cut -c 1 | tr '[:upper:]' '[:lower:]')
  path_without_drive=${windows_path#?:}
  printf '/cygdrive/%s%s\n' "$drive_letter" "$path_without_drive"
}

run_rsync() {
  if [ "$WINDOWS_RSYNC" = true ]; then
    # 防止 MSYS 将 /cygdrive/d/... 再次转换为会被 rsync 误判的 D:/...。
    MSYS2_ARG_CONV_EXCL='*' rsync "$@"
  else
    rsync "$@"
  fi
}

for command_name in git rsync mktemp; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "错误：缺少必要命令：$command_name" >&2
    exit 1
  fi
done

if [ "$WINDOWS_RSYNC" = true ] && ! command -v cygpath >/dev/null 2>&1; then
  echo "错误：Windows 环境下缺少必要命令：cygpath" >&2
  exit 1
fi

if ! git -C "$SOURCE_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "错误：当前目录不是 Git 工作区：$SOURCE_DIR" >&2
  exit 1
fi

for project_dir in "$BACKEND_DIR" "$FRONTEND_DIR"; do
  if [ ! -d "$SOURCE_DIR/$project_dir" ]; then
    echo "错误：当前目录缺少 $project_dir" >&2
    exit 1
  fi
done

if [ "$SOURCE_DIR" = "$BACKUP_DIR" ]; then
  echo "错误：不能在备份目录自身运行此脚本" >&2
  exit 1
fi

FILE_LIST=$(mktemp)
trap 'rm -f -- "$FILE_LIST"' EXIT HUP INT TERM

RSYNC_SOURCE_DIR=$(to_rsync_path "$SOURCE_DIR")
RSYNC_BACKUP_DIR=$(to_rsync_path "$BACKUP_DIR")
RSYNC_FILE_LIST=$(to_rsync_path "$FILE_LIST")

# 先复制两个子项目之外的所有内容。
mkdir -p -- "$BACKUP_DIR"
run_rsync -a \
  --exclude="/$BACKEND_DIR/" \
  --exclude="/$FRONTEND_DIR/" \
  "$RSYNC_SOURCE_DIR/" "$RSYNC_BACKUP_DIR/"

# Git 负责处理 .gitignore 的目录规则、通配符及 ! 否定规则。
# --cached 会保留已被 Git 跟踪的文件，即使它后来匹配了忽略规则。
git -C "$SOURCE_DIR" ls-files -z \
  --cached \
  --others \
  --exclude-per-directory=.gitignore \
  -- "$BACKEND_DIR" "$FRONTEND_DIR" >"$FILE_LIST"

run_rsync -a \
  --from0 \
  --files-from="$RSYNC_FILE_LIST" \
  "$RSYNC_SOURCE_DIR/" "$RSYNC_BACKUP_DIR/"

echo "备份完成：$BACKUP_DIR"
