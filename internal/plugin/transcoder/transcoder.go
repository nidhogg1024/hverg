package transcoder

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/nidhogg1024/hverg/internal/plugin"
)

func init() {
	plugin.Register("grpc_transcoder", NewTranscoderPlugin)
}

// TranscoderPlugin intercepts HTTP requests and translates them to gRPC calls
// using dynamic protobuf descriptors.
type TranscoderPlugin struct {
	ProtoService   string
	ProtoMethod    string
	DescriptorFile string
	Backend        string

	methodDesc protoreflect.MethodDescriptor
	conn       *grpc.ClientConn
	resolver   *dynamicpb.Types
}

// NewTranscoderPlugin creates a new transcoder plugin instance.
func NewTranscoderPlugin(cfg map[string]interface{}) (plugin.Plugin, error) {
	t := &TranscoderPlugin{}

	if ps, ok := cfg["proto_service"].(string); ok {
		t.ProtoService = ps
	}
	if pm, ok := cfg["proto_method"].(string); ok {
		t.ProtoMethod = pm
	}
	if df, ok := cfg["descriptor_file"].(string); ok {
		t.DescriptorFile = df
	}
	if be, ok := cfg["_route_backend"].(string); ok {
		t.Backend = be
	}

	if t.DescriptorFile == "" {
		return nil, fmt.Errorf("descriptor_file is required for grpc_transcoder")
	}

	// 1. Load the descriptor set from file
	b, err := os.ReadFile(t.DescriptorFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read descriptor file %s: %w", t.DescriptorFile, err)
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal descriptor set: %w", err)
	}

	// 2. Build a registry from the descriptor set
	files, err := protodesc.NewFiles(&fds)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry from descriptor set: %w", err)
	}
	t.resolver = dynamicpb.NewTypes(files)

	// 3. Find the method descriptor
	// Service name needs to be fully qualified, e.g., "order.v1.OrderService"
	serviceName := protoreflect.FullName(t.ProtoService)
	desc, err := files.FindDescriptorByName(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to find service %s: %w", t.ProtoService, err)
	}

	serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a service", t.ProtoService)
	}

	methodDesc := serviceDesc.Methods().ByName(protoreflect.Name(t.ProtoMethod))
	if methodDesc == nil {
		return nil, fmt.Errorf("method %s not found in service %s", t.ProtoMethod, t.ProtoService)
	}
	t.methodDesc = methodDesc

	// 4. Establish gRPC connection to backend
	// Remove "grpc://" prefix if present
	target := strings.TrimPrefix(t.Backend, "grpc://")
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to grpc backend %s: %w", target, err)
	}
	t.conn = conn

	return t, nil
}

// Name returns the plugin name.
func (p *TranscoderPlugin) Name() string {
	return "grpc_transcoder"
}

// Handle executes the dynamic gRPC translation logic.
func (p *TranscoderPlugin) Handle(ctx *plugin.Context) error {
	slog.Info("Executing gRPC transcoder plugin",
		"service", p.ProtoService,
		"method", p.ProtoMethod,
		"path", ctx.Request.URL.Path,
	)

	// 1. Read JSON body from HTTP request
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.AbortWithStatusJSON(400, []byte(`{"error": "failed to read request body"}`))
		return nil
	}
	defer ctx.Request.Body.Close()

	// 2. Create dynamic request message and unmarshal JSON
	reqMsg := dynamicpb.NewMessage(p.methodDesc.Input())
	
	// If body is empty, treat it as empty JSON object
	if len(body) == 0 {
		body = []byte("{}")
	}

	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
		Resolver:       p.resolver,
	}
	if err := unmarshaler.Unmarshal(body, reqMsg); err != nil {
		slog.Error("Failed to unmarshal JSON to protobuf", "err", err)
		ctx.AbortWithStatusJSON(400, []byte(fmt.Sprintf(`{"error": "invalid json format: %v"}`, err)))
		return nil
	}

	// 3. Invoke gRPC method dynamically
	respMsg := dynamicpb.NewMessage(p.methodDesc.Output())
	invokePath := fmt.Sprintf("/%s/%s", p.ProtoService, p.ProtoMethod)

	// Pass context from HTTP request
	err = p.conn.Invoke(ctx.Request.Context(), invokePath, reqMsg, respMsg)
	if err != nil {
		slog.Error("gRPC invocation failed", "path", invokePath, "err", err)
		ctx.AbortWithStatusJSON(500, []byte(fmt.Sprintf(`{"error": "upstream grpc error: %v"}`, err)))
		return nil
	}

	// 4. Serialize dynamic response message back to JSON
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		Resolver:        p.resolver,
	}
	respJSON, err := marshaler.Marshal(respMsg)
	if err != nil {
		slog.Error("Failed to marshal protobuf to JSON", "err", err)
		ctx.AbortWithStatusJSON(500, []byte(`{"error": "failed to encode response"}`))
		return nil
	}

	// 5. Write JSON response to client
	ctx.AbortWithStatusJSON(200, respJSON)

	return nil
}
