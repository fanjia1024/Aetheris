#!/usr/bin/env bash
# 端到端测试脚本：健康检查 → 上传文件 → 列出文档 → 发送查询
# 用法: ./scripts/test-e2e.sh [文件路径] [查询问题]
# 示例: ./scripts/test-e2e.sh ./AGENTS.md "What is this document about?"
# 默认: 文件路径为 ./AGENTS.md，查询为 "Summarize the main content."

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
FILE_PATH="${1:-./AGENTS.md}"
QUERY="${2:-Summarize the main content.}"

if [[ ! -f "$FILE_PATH" ]]; then
  echo "Error: file not found: $FILE_PATH"
  exit 1
fi

echo "=== 1. Health check ==="
health=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/health")
if [[ "$health" != "200" ]]; then
  echo "Health check failed (HTTP $health). Is the API running at $BASE_URL?"
  exit 1
fi
echo "OK (HTTP $health)"

echo ""
echo "=== 2. Upload document: $FILE_PATH ==="
upload_resp=$(curl -s -X POST "$BASE_URL/api/documents/upload" -F "file=@$FILE_PATH")
echo "$upload_resp" | head -c 500
echo ""
if echo "$upload_resp" | grep -q '"doc_id"\|"document_id"\|"id"'; then
  echo "Upload response contains doc id / chunks (check above)."
else
  echo "Upload response (check for errors above)."
fi

echo ""
echo "=== 3. List documents ==="
list_resp=$(curl -s "$BASE_URL/api/documents/")
echo "$list_resp" | head -c 400
echo ""
echo "..."

echo ""
echo "=== 4. Query: $QUERY ==="
query_resp=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"$QUERY\", \"top_k\": 10}")
echo "$query_resp" | head -c 600
echo ""
echo "..."

echo ""
echo "Done. Review the responses above for doc_id, chunks, and answer."
