```bash
# 新增应用目录
mkdir -p /opt/apps/ocopion-backend
# 打包并上传到服务器应用目录，deploy.sh 中先注释 systemctl 操作
sh scripts/deploy.sh
# 上传ocopion.service
rsync -avz --progress --partial ./scripts/ocopion.service qyhever:/etc/systemd/system/ocopion.service
systemctl start ocopion
systemctl enable ocopion
systemctl status ocopion
# 可以放开 deploy.sh systemctl 操作了
```