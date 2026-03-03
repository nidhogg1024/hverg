package transcoder

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// descEntry 存储解析后的 Protobuf 文件注册表和动态类型解析器
type descEntry struct {
	files *protoregistry.Files
	types *dynamicpb.Types
}

var globalDescCache = &descCache{
	entries: make(map[string]*descEntry),
}

// descCache 缓存已加载的 Protobuf 描述符文件，避免同一个 .desc 文件被重复解析。
type descCache struct {
	mu      sync.RWMutex
	entries map[string]*descEntry
}

// GetOrLoad 从缓存获取描述符注册表，如果不存在则从文件加载。
func (c *descCache) GetOrLoad(filePath string) (*descEntry, error) {
	c.mu.RLock()
	if entry, ok := c.entries[filePath]; ok {
		c.mu.RUnlock()
		return entry, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check
	if entry, ok := c.entries[filePath]; ok {
		return entry, nil
	}

	slog.Info("Loading protobuf descriptor file", "path", filePath)

	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read descriptor file %s: %w", filePath, err)
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal descriptor set from %s: %w", filePath, err)
	}

	files, err := protodesc.NewFiles(&fds)
	if err != nil {
		return nil, fmt.Errorf("failed to create file registry from %s: %w", filePath, err)
	}

	entry := &descEntry{
		files: files,
		types: dynamicpb.NewTypes(files),
	}

	c.entries[filePath] = entry
	return entry, nil
}

// FindMethod 在指定的描述符条目中查找 gRPC 方法描述符
func (e *descEntry) FindMethod(serviceName, methodName string) (protoreflect.MethodDescriptor, error) {
	desc, err := e.files.FindDescriptorByName(protoreflect.FullName(serviceName))
	if err != nil {
		return nil, fmt.Errorf("service %s not found: %w", serviceName, err)
	}

	serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a service descriptor", serviceName)
	}

	method := serviceDesc.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	return method, nil
}
