package transcoder

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/nidhogg1024/hverg/internal/plugin"
)

func init() {
	plugin.Register("grpc_transcoder", NewTranscoderPlugin)
}

// TranscoderPlugin 拦截 HTTP 请求，通过动态 Protobuf 描述符将其转换为 gRPC 泛化调用。
type TranscoderPlugin struct {
	protoService string
	protoMethod  string

	methodDesc protoreflect.MethodDescriptor
	conn       *grpc.ClientConn
	resolver   *dynamicpb.Types

	// 预分配的 protojson 选项，避免每次请求重新构造
	unmarshalOpts protojson.UnmarshalOptions
	marshalOpts   protojson.MarshalOptions
}

// NewTranscoderPlugin 通过配置创建 Transcoder 插件实例。
// 使用全局的连接池和描述符缓存，避免重复创建。
func NewTranscoderPlugin(cfg map[string]interface{}) (plugin.Plugin, error) {
	protoService, _ := cfg["proto_service"].(string)
	protoMethod, _ := cfg["proto_method"].(string)
	descriptorFile, _ := cfg["descriptor_file"].(string)
	backend, _ := cfg["_route_backend"].(string)

	if descriptorFile == "" {
		return nil, fmt.Errorf("grpc_transcoder: descriptor_file is required")
	}
	if protoService == "" || protoMethod == "" {
		return nil, fmt.Errorf("grpc_transcoder: proto_service and proto_method are required")
	}

	// 从全局缓存加载描述符（如果已经加载过则直接命中）
	entry, err := globalDescCache.GetOrLoad(descriptorFile)
	if err != nil {
		return nil, err
	}

	// 在描述符注册表中查找目标 gRPC 方法
	methodDesc, err := entry.FindMethod(protoService, protoMethod)
	if err != nil {
		return nil, fmt.Errorf("grpc_transcoder: %w", err)
	}

	// 从全局连接池获取/创建到后端的 gRPC 连接
	target := strings.TrimPrefix(backend, "grpc://")
	conn, err := globalConnPool.GetConn(target)
	if err != nil {
		return nil, err
	}

	return &TranscoderPlugin{
		protoService: protoService,
		protoMethod:  protoMethod,
		methodDesc:   methodDesc,
		conn:         conn,
		resolver:     entry.types,
		unmarshalOpts: protojson.UnmarshalOptions{
			DiscardUnknown: true,
			Resolver:       entry.types,
		},
		marshalOpts: protojson.MarshalOptions{
			EmitUnpopulated: true,
			Resolver:        entry.types,
		},
	}, nil
}

func (p *TranscoderPlugin) Name() string {
	return "grpc_transcoder"
}

// Handle 执行动态 gRPC 转译逻辑：
// JSON Body -> dynamicpb.Message -> gRPC Invoke -> dynamicpb.Message -> JSON Response
func (p *TranscoderPlugin) Handle(ctx *plugin.Context) error {
	// 1. 读取 HTTP 请求的 JSON Body
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.AbortWithStatusJSON(400, []byte(`{"error":"failed to read request body"}`))
		return nil
	}
	defer ctx.Request.Body.Close()

	if len(body) == 0 {
		body = []byte("{}")
	}

	// 2. 利用描述符将 JSON 反序列化为动态 Protobuf Message
	reqMsg := dynamicpb.NewMessage(p.methodDesc.Input())
	if err := p.unmarshalOpts.Unmarshal(body, reqMsg); err != nil {
		slog.Error("JSON -> Protobuf unmarshal failed", "err", err, "service", p.protoService, "method", p.protoMethod)
		ctx.AbortWithStatusJSON(400, []byte(fmt.Sprintf(`{"error":"invalid json: %v"}`, err)))
		return nil
	}

	// 3. 发起 gRPC 泛化调用
	respMsg := dynamicpb.NewMessage(p.methodDesc.Output())
	fullMethod := fmt.Sprintf("/%s/%s", p.protoService, p.protoMethod)

	if err := p.conn.Invoke(ctx.Request.Context(), fullMethod, reqMsg, respMsg); err != nil {
		slog.Error("gRPC invoke failed", "method", fullMethod, "err", err)
		ctx.AbortWithStatusJSON(502, []byte(fmt.Sprintf(`{"error":"upstream grpc error: %v"}`, err)))
		return nil
	}

	// 4. 将响应 Protobuf Message 序列化为 JSON 返回给客户端
	respJSON, err := p.marshalOpts.Marshal(respMsg)
	if err != nil {
		slog.Error("Protobuf -> JSON marshal failed", "err", err)
		ctx.AbortWithStatusJSON(500, []byte(`{"error":"failed to encode response"}`))
		return nil
	}

	ctx.AbortWithStatusJSON(200, respJSON)
	return nil
}
