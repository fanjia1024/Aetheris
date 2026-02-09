// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package grpc 提供 gRPC 服务端，与 HTTP 能力对齐；调用 Engine 与 DocumentService，不直接调 storage。
package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	appcore "rag-platform/internal/app"
	"rag-platform/internal/api/grpc/pb"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/runtime/eino"
)

// Server gRPC 服务端，持有 Engine 与 DocumentService
type Server struct {
	pb.UnimplementedDocumentServiceServer
	pb.UnimplementedQueryServiceServer
	engine    *eino.Engine
	docService appcore.DocumentService
}

// NewServer 根据注入的 Engine 与 DocumentService 创建 gRPC Server
func NewServer(engine *eino.Engine, docService appcore.DocumentService) *Server {
	return &Server{
		engine:    engine,
		docService: docService,
	}
}

// Register 注册 Document 与 Query 服务到 grpc.Server
func (s *Server) Register(grpcServer *grpc.Server) {
	pb.RegisterDocumentServiceServer(grpcServer, s)
	pb.RegisterQueryServiceServer(grpcServer, s)
}

// ListDocuments 实现 DocumentService.ListDocuments
func (s *Server) ListDocuments(ctx context.Context, req *pb.ListDocumentsRequest) (*pb.ListDocumentsResponse, error) {
	docs, err := s.docService.ListDocuments(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list documents: %v", err)
	}
	out := make([]*pb.DocumentInfo, len(docs))
	for i, d := range docs {
		out[i] = docInfoToPB(d)
	}
	return &pb.ListDocumentsResponse{
		Documents: out,
		Total:     int32(len(out)),
	}, nil
}

// GetDocument 实现 DocumentService.GetDocument
func (s *Server) GetDocument(ctx context.Context, req *pb.GetDocumentRequest) (*pb.GetDocumentResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	doc, err := s.docService.GetDocument(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get document: %v", err)
	}
	return &pb.GetDocumentResponse{Document: docInfoToPB(doc)}, nil
}

// DeleteDocument 实现 DocumentService.DeleteDocument
func (s *Server) DeleteDocument(ctx context.Context, req *pb.DeleteDocumentRequest) (*pb.DeleteDocumentResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	if err := s.docService.DeleteDocument(ctx, req.GetId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete document: %v", err)
	}
	return &pb.DeleteDocumentResponse{Success: true}, nil
}

// UploadDocument 实现 DocumentService.UploadDocument（通过 ingest_pipeline，params 使用 content []byte）
func (s *Server) UploadDocument(ctx context.Context, req *pb.UploadDocumentRequest) (*pb.UploadDocumentResponse, error) {
	if len(req.GetContent()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "content required")
	}
	result, err := s.engine.ExecuteWorkflow(ctx, "ingest_pipeline", map[string]interface{}{
		"content": req.GetContent(),
		"metadata": map[string]interface{}{
			"filename":     req.GetFilename(),
			"content_type": req.GetContentType(),
			"uploaded_at":  time.Now(),
		},
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "upload document: %v", err)
	}
	m, _ := result.(map[string]interface{})
	docID, _ := m["doc_id"].(string)
	return &pb.UploadDocumentResponse{
		Success:    true,
		Message:    "文档上传成功",
		DocumentId: docID,
	}, nil
}

// Query 实现 QueryService.Query
func (s *Server) Query(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query required")
	}
	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = 10
	}
	q := &common.Query{
		ID:        fmt.Sprintf("query-%d", time.Now().UnixNano()),
		Text:      req.GetQuery(),
		Metadata:  nil,
		CreatedAt: time.Now(),
	}
	if req.Metadata != nil {
		q.Metadata = make(map[string]interface{})
		for k, v := range req.Metadata {
			q.Metadata[k] = v
		}
	}
	result, err := s.engine.ExecuteWorkflow(ctx, "query_pipeline", map[string]interface{}{
		"query": q,
		"top_k": topK,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query: %v", err)
	}
	genResult, ok := result.(*common.GenerationResult)
	if !ok {
		return &pb.QueryResponse{Success: true, Answer: fmt.Sprint(result)}, nil
	}
	return &pb.QueryResponse{
		Success:    true,
		Answer:     genResult.Answer,
		References: genResult.References,
	}, nil
}

// BatchQuery 实现 QueryService.BatchQuery
func (s *Server) BatchQuery(ctx context.Context, req *pb.BatchQueryRequest) (*pb.BatchQueryResponse, error) {
	results := make([]*pb.QueryResponse, 0, len(req.GetQueries()))
	for _, q := range req.GetQueries() {
		res, err := s.Query(ctx, q)
		if err != nil {
			results = append(results, &pb.QueryResponse{Success: false, Error: err.Error()})
			continue
		}
		results = append(results, res)
	}
	return &pb.BatchQueryResponse{Results: results}, nil
}

func docInfoToPB(d *appcore.DocumentInfo) *pb.DocumentInfo {
	if d == nil {
		return nil
	}
	return &pb.DocumentInfo{
		Id:          d.ID,
		Name:        d.Name,
		Type:        d.Type,
		Size:        d.Size,
		Path:        d.Path,
		Status:      d.Status,
		Chunks:      int32(d.Chunks),
		VectorCount: int32(d.VectorCount),
		Metadata:    d.Metadata,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}
