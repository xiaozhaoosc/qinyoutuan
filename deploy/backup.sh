#!/bin/bash
# Daily backup of the qinyoutuan SQLite database. Keeps 7 days.
# Install: cp to /opt/qinyoutuan/backup.sh, chmod +x, add the cron in deploy/README.md.
set -e
DIR=/opt/qinyoutuan/backups
mkdir -p "$DIR"
TS=$(date +%Y%m%d-%H%M%S)

# Online hot backup (safe while the app is running).
sqlite3 /opt/qinyoutuan/qinyoutuan.db ".backup '$DIR/qinyoutuan-$TS.db'"

# Retention: delete backups older than 7 days.
find "$DIR" -maxdepth 1 -name '*.db' -mtime +7 -delete 2>/dev/null || true
echo "$(date '+%F %T') backup ok -> $DIR (qinyoutuan-$TS.db)"
