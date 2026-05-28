# 部署

> 以下命令均在 `backend/` 目录下执行。

## 配置

```bash
# 替换为实际值
IMAGE=cs-hub.imgo.tv/library/your-project:v1.0
CONTAINER=your-project
```

---

## Dockerfile

多阶段构建：`golang1.25` 编译，`centos-hb:v1` 运行。

---

## 构建 & 推送

```bash
docker build -t $IMAGE .
docker push $IMAGE
```

---

## 运行

```bash
# 启动
docker run -d \
  --name $CONTAINER \
  --restart unless-stopped \
  --network host \
  -v /data:/data \
  -e APP_ENV=prod \
  $IMAGE

# 调试（进入容器 shell）
docker run --rm \
  --network host \
  -v /data:/data \
  -e APP_ENV=prod \
  --entrypoint "" \
  -ti $IMAGE /bin/sh
```

---

## 运维

```bash
docker logs -f $CONTAINER               # 查看日志
docker exec -ti $CONTAINER /bin/sh      # 进入容器
docker restart $CONTAINER               # 重启
docker stop $CONTAINER && docker rm $CONTAINER  # 停止并删除
```
