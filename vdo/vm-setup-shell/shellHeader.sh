#!/usr/bin/env bash
set -euo pipefail

LOG=/var/log/ark-init.log
exec 3>&1                       # anything we want Azure to show
exec 1>>"$LOG" 2>&1             # everything else into a file

on_error() {
  local exit_code=$?
  local line_no=$1
  local cmd=$2
  printf '[%s] [ERROR] line %d → "%s" (exit %d)\n' \
         "$(date +'%F %T')" "$line_no" "$cmd" "$exit_code" >&3
  exit "$exit_code"
}
set -o errtrace           # let ERR propagate through functions/ subshells
trap 'on_error $LINENO "$BASH_COMMAND"' ERR

log(){ printf '[%s] [INFO] %s\n' "$(date +'%F %T')" "$*" >&3; }

