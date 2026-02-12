-- k6 Benchmark for simple jobs (single LLM call)
-- Usage: k6 run simple_job.lua

wrk.method = "POST"
wrk.body   = '{"agent_id":"test-agent","goal":"Simple task","session_id":"bench-session"}'
wrk.headers["Content-Type"] = "application/json"

function response(status, headers, body)
   if status ~= 200 and status ~= 202 then
      print("Error: " .. status .. " " .. body)
   end
end
