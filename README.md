# Hverg (Hvergelmir Gateway)

一个轻量级、插件化的 API 网关，支持 HTTP 反向代理透传与基于动态 Protobuf 描述符的 HTTP-to-gRPC 泛化调用。

> **Hvergelmir**（赫瓦格密尔）是北欧神话中世界之树根下的"沸腾之泉"，所有河流的源头，也是黑龙 Nidhogg 栖息之地。
> Hverg 隐喻了"一切请求的起点"——微服务架构的流量之源。

---

## 核心特性

- **极简 HTTP 反向代理**：零配置将 HTTP/JSON 请求透传到下游 HTTP 后端，开箱即用。
- **动态 gRPC 转译 (Transcoder)**：通过加载 `.desc` Protobuf 描述符文件，在运行时将 HTTP/JSON 请求动态翻译为 gRPC 调用——无需生成任何 Stub 代码，无需重新编译网关。
- **插件化架构 (Plugin Pipeline)**：鉴权、限流、协议转换等所有功能均以插件形式按路由挂载，按需组合。
- **声明式 YAML 配置**：一份配置文件定义全部路由与插件链，清晰易读。
- **优雅停机 (Graceful Shutdown)**：支持信号驱动的平滑关闭，保证在途请求处理完毕。

---

## 架构总览

```
                        ┌─────────────────────────────────────────┐
                        │            Hverg Gateway                │
  Client ──HTTP/JSON──► │  Router ──► Plugin Chain ──► Backend    │
                        │             (Auth, Transcoder, ...)     │
                        └──────────────┬──────────────┬───────────┘
                                       │              │
                              HTTP Proxy (透传)   gRPC Invoke (泛化调用)
                                       │              │
                                       ▼              ▼
                              ┌──────────────┐ ┌──────────────┐
                              │ HTTP Service │ │ gRPC Service │
                              └──────────────┘ └──────────────┘
```

---

## 快速开始

### 前置要求

- Go 1.22+
- [Buf CLI](https://buf.build/docs/installation)（仅在需要生成 `.desc` 文件时使用）

### 1. 构建

```bash
git clone https://github.com/nidhogg1024/hverg.git
cd hverg
go build -o bin/hverg cmd/hverg/main.go
```

### 2. 配置

创建或编辑 `hverg.yaml`：

```yaml
server:
  addr: ":8080"

routes:
  # 场景一：纯 HTTP 透传
  - path: /api/v1/users
    method: GET
    backend: http://localhost:8081
    plugins:
      - name: jwt_auth
        config:
          header_name: Authorization
          secret: "my-secret-key"

  # 场景二：HTTP -> gRPC 动态转译
  - path: /api/v2/orders
    method: POST
    backend: grpc://localhost:9090
    plugins:
      - name: grpc_transcoder
        config:
          proto_service: order.v1.OrderService
          proto_method: CreateOrder
          descriptor_file: testdata/pb/order.desc
```

### 3. 启动

```bash
./bin/hverg -config hverg.yaml
```

---

## gRPC 动态转译说明

Hverg 的核心卖点之一：**无需编译任何 Stub 代码，即可将 HTTP/JSON 请求转换为 gRPC 调用**。

### 工作原理

1. 使用 `buf build` 或 `protoc --descriptor_set_out` 将 `.proto` 文件编译为二进制描述符文件 (`.desc`)。
2. 网关启动时加载 `.desc` 文件，构建内存中的 Protobuf 类型注册表。
3. 收到 HTTP 请求后，利用 `dynamicpb` + `protojson` 将 JSON Body 动态反序列化为 Protobuf Message。
4. 通过 `grpc.Invoke` 发起泛化调用，再将响应序列化回 JSON 返回给客户端。

### 生成描述符文件

```bash
# 使用 Buf（推荐）
buf build testdata/pb -o testdata/pb/order.desc

# 或使用 protoc
protoc --descriptor_set_out=order.desc --include_imports testdata/pb/order.proto
```

### 测试

```bash
# HTTP 透传测试（需要后端在 8081 端口运行一个 HTTP 服务）
curl http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer valid-mock-token"

# gRPC 转译测试（需要后端在 9090 端口运行一个 gRPC 服务）
curl -X POST http://localhost:8080/api/v2/orders \
  -H "Content-Type: application/json" \
  -d '{"user_id": "u123", "item_id": "item456", "quantity": 2}'
```

---

## 项目结构

```
hverg/
├── cmd/hverg/                  # 网关入口
│   └── main.go
├── internal/
│   ├── config/                 # YAML 配置加载与定义
│   │   ├── config.go
│   │   └── loader.go
│   ├── plugin/                 # 插件引擎与接口定义
│   │   ├── plugin.go           # Plugin 接口、Chain、Registry
│   │   ├── auth/               # JWT 鉴权插件
│   │   │   └── jwt.go
│   │   └── transcoder/         # gRPC 动态转译插件
│   │       └── transcoder.go
│   ├── proxy/                  # HTTP 反向代理封装
│   │   └── proxy.go
│   └── router/                 # 路由引擎
│       └── router.go
├── testdata/pb/                # 测试用 Protobuf 文件
│   └── order.proto
├── hverg.yaml                  # 示例配置文件
├── go.mod
└── go.sum
```

---

## 内置插件

| 插件名 | 说明 | 关键配置项 |
|--------|------|-----------|
| `jwt_auth` | JWT / Bearer Token 鉴权 | `header_name`, `secret` |
| `grpc_transcoder` | HTTP/JSON -> gRPC 动态转译 | `proto_service`, `proto_method`, `descriptor_file` |

---

## 设计哲学

- **不做 BFF**：Hverg 不做业务数据的聚合与裁剪，只做统一接入、鉴权和协议转换。复杂的组装逻辑交给下游的 BFF 服务。
- **网关越薄越稳**：核心基座只是一个极简的反向代理，所有增值功能通过插件挂载，保证基座的极高稳定性。
- **动态优于静态**：通过运行时加载 Protobuf 描述符实现泛化调用，下游新增接口无需重新编译网关，推送配置即可生效。

---

## Roadmap

- [ ] 真实 JWT 签名验证（集成 `golang-jwt/jwt`）
- [ ] gRPC 连接池与负载均衡
- [ ] 配置热重载（文件监听 / etcd / Consul）
- [ ] 更多内置插件：限流、熔断、请求日志、跨域 (CORS)
- [ ] 管理 API / 控制面
- [ ] 完整的集成测试套件

---

## License

MIT
