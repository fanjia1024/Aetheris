// Package grpc 提供 gRPC 服务端占位，与 http 并列；设计见 struct.md 3.7。
//
// 后续可在此包内：
//   - 定义或生成与 HTTP 能力对齐的 gRPC 服务（文档列表/获取/删除、上传、查询）
//   - 仅调用 runtime / pipeline 门面，不直接调 storage（与 Handler 一致）
//   - 对接 proto 与 eino，供 api-service → agent-service 内部调用（services.md）
package grpc

// Server 占位：后续作为 gRPC 服务端，注入 *eino.Engine 与 DocumentService（或等价门面）。
type Server struct {
	// Engine   *eino.Engine
	// DocSvc   DocumentService
	// Logger   *log.Logger
}

// NewServer 占位：后续根据配置与注入的 Engine/DocumentService 创建 gRPC Server。
func NewServer() *Server {
	return &Server{}
}

// Register 占位：后续注册 gRPC 服务（如 pb.RegisterDocumentServiceServer）。
// func (s *Server) Register(grpcServer *grpc.Server) {}
