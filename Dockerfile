# 注意：不加 "# syntax=docker/dockerfile:1"——本文件只用内置 frontend 支持的基础指令，
# 弱网下该指令还要去 docker.io 拉 frontend 镜像（auth.docker.io 超时，实测踩坑），无收益
# ============ 可参数化基座（弱网换源用，对齐前端 Dockerfile 的 ARG 模式）============
# 弱网：docker pull docker.m.daocloud.io/library/golang:1.24-alpine && docker tag ... golang:1.24-alpine
#      或 docker build --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.24-alpine .
#      RUNTIME_IMAGE 同法（见 plans/PLAN-docker.md 实测记录的弱网先例）
ARG GO_IMAGE=golang:1.24-alpine
ARG RUNTIME_IMAGE=alpine:3.21

# ============ 构建阶段 ============
FROM ${GO_IMAGE} AS builder
# FROM 之前声明的 ARG 只在 FROM 行生效，stage 内使用需重新声明（继承同名默认值）
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src

# 层缓存锚点：依赖清单不变则命中 go mod download 层，改业务代码不重下依赖
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 静态编译（全依赖纯 Go）；-trimpath 去本机路径，-s -w 去 symbol/DWARF 减体积；
# 单 builder 产出双二进制，compose 里 consumer 服务用 command 覆盖启动另一个（见 PLAN-docker.md）
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/consumer ./cmd/consumer

# ============ 运行阶段 ============
FROM ${RUNTIME_IMAGE}
# tzdata：DSN 是 Loc: time.Local（internal/config/config.go），无 tzdata 时容器内 time.Local=UTC，
#   DATETIME 比 mysql 容器（TZ=Asia/Shanghai）偏 8h
# ca-certificates：小鹅通透传 https://api.xiaoe-tech.com 必需，裸 alpine 会 certificate is not trusted
RUN apk add --no-cache tzdata ca-certificates
ENV TZ=Asia/Shanghai

# 版本溯源（对齐前端 GoProject-web Dockerfile）：语义 tag 不含 hash，代码对应关系靠这两个 ENV，
# tag 被 prune 后 docker inspect 仍可查。仅 deploy.sh 的 docker build 传精确值；
# compose 日常 up -d --build 路径不传，落默认值 dev/unknown，天然区分「手动构建」与「发版构建」
ARG IMAGE_TAG=dev
ARG GIT_REV=unknown
ENV IMAGE_TAG=${IMAGE_TAG}
ENV GIT_REV=${GIT_REV}

WORKDIR /app
COPY --from=builder /out/server   /app/server
COPY --from=builder /out/consumer /app/consumer

EXPOSE 8080
# 用 CMD 而非 ENTRYPOINT：compose 的 consumer 服务才能用 command: ["/app/consumer"] 整条覆盖。
# HEALTHCHECK 只写 compose 不写镜像：镜像被 server/consumer 共用，镜像级探活会在 consumer 容器上永远 unhealthy
CMD ["/app/server"]
