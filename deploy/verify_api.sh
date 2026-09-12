#!/bin/bash
# 部署验证脚本：在 NAS 上执行，验证后端 API 与前端页面
echo "==== 1. 登录 admin 获取 token ===="
LOGIN=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"password"}')
echo "$LOGIN" | head -c 300
echo
TOKEN=$(echo "$LOGIN" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
echo "TOKEN_LEN=${#TOKEN}"

echo
echo "==== 2. GET /api/routes (带 token) ===="
curl -s -w '\nHTTP_CODE=%{http_code}\n' \
  -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:8080/api/routes | head -100

echo
echo "==== 3. 前端 http://127.0.0.1:8000/ ===="
curl -s -o /dev/null -w 'HTTP_CODE=%{http_code}\n' http://127.0.0.1:8000/
curl -s http://127.0.0.1:8000/ | head -25
