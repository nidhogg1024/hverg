package auth

import (
	"log/slog"
	"strings"

	"github.com/nidhogg1024/hverg/internal/plugin"
)

func init() {
	plugin.Register("jwt_auth", NewJWTAuthPlugin)
}

// JWTAuthPlugin is a simple mock JWT authentication plugin
type JWTAuthPlugin struct {
	HeaderName string
	Secret     string
}

// NewJWTAuthPlugin creates a new JWT auth plugin from configuration
func NewJWTAuthPlugin(cfg map[string]interface{}) (plugin.Plugin, error) {
	headerName := "Authorization"
	if h, ok := cfg["header_name"].(string); ok && h != "" {
		headerName = h
	}

	secret := ""
	if s, ok := cfg["secret"].(string); ok {
		secret = s
	}

	return &JWTAuthPlugin{
		HeaderName: headerName,
		Secret:     secret,
	}, nil
}

// Name returns the plugin name
func (p *JWTAuthPlugin) Name() string {
	return "jwt_auth"
}

// Handle executes the authentication logic
func (p *JWTAuthPlugin) Handle(ctx *plugin.Context) error {
	token := ctx.Request.Header.Get(p.HeaderName)

	if token == "" {
		ctx.AbortWithStatusJSON(401, []byte(`{"error": "unauthorized", "message": "missing token"}`))
		return nil
	}

	// For standard Bearer token
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	// TODO: 替换为真实的 JWT 签名验证逻辑（如 golang-jwt/jwt）
	if p.Secret != "" && token != "valid-mock-token" {
		slog.Warn("Invalid token", "token", token)
		ctx.AbortWithStatusJSON(401, []byte(`{"error": "unauthorized", "message": "invalid token"}`))
		return nil
	}

	// 将解析出的用户信息写入 Context，后续插件可通过 ctx.GetString("user_id") 获取
	ctx.Set("user_id", "mock-user-id")
	ctx.Set("authenticated", true)

	// 同时写入 HTTP Header，以便下游 HTTP 后端也能获取到鉴权信息
	ctx.Request.Header.Set("X-User-Id", "mock-user-id")

	return nil
}
