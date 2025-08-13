#!/bin/bash

# Generate test JSON logs matching current logrus flat structure
LOG_FILE="./logs/simple-proxy.jsonl"

# Ensure logs directory exists
mkdir -p logs

# Generate structured test logs with flat JSON structure (no nested fields)
cat >> "$LOG_FILE" << 'EOF'
{"timestamp":"2025-08-13T10:30:00.000Z","level":"info","service":"simple-proxy","component":"proxy_core","category":"request","request_id":"req_test_001","message":"Test request initiated","user_agent":"test-client","endpoint":"/v1/messages"}
{"timestamp":"2025-08-13T10:30:00.100Z","level":"info","service":"simple-proxy","component":"hybrid_classifier","category":"classification","request_id":"req_test_001","message":"Tool necessity decision","decision":"require_tools","reason":"Strong implementation verb detected","confident":true,"verb":"create","artifact":"file.go","stage":"B_rule_engine","rule_name":"StrongVerbWithFile","matched":true}
{"timestamp":"2025-08-13T10:30:00.200Z","level":"info","service":"simple-proxy","component":"tool_correction","category":"transformation","request_id":"req_test_001","message":"Parameter correction applied","tool_name":"Read","original_param":"filename","corrected_param":"file_path"}
{"timestamp":"2025-08-13T10:30:00.300Z","level":"info","service":"simple-proxy","component":"circuit_breaker","category":"health","request_id":"req_test_001","message":"Endpoint healthy","endpoint":"http://192.168.0.46:11434","success_rate":0.95,"failure_count":0}
{"timestamp":"2025-08-13T10:30:00.400Z","level":"error","service":"simple-proxy","component":"tool_correction","category":"error","request_id":"req_test_001","message":"Tool correction failed","tool_name":"BadTool","error":"invalid parameters","retry_count":3}
{"timestamp":"2025-08-13T10:30:01.000Z","level":"info","service":"simple-proxy","component":"proxy_core","category":"request","request_id":"req_test_002","message":"Test streaming request","model":"claude-sonnet-4","tokens":1500}
{"timestamp":"2025-08-13T10:30:01.200Z","level":"warn","service":"simple-proxy","component":"circuit_breaker","category":"warning","request_id":"req_test_002","message":"Endpoint showing latency","endpoint":"http://192.168.0.50:11434","response_time_ms":2500}
{"timestamp":"2025-08-13T10:30:01.500Z","level":"info","service":"simple-proxy","component":"hybrid_classifier","category":"classification","request_id":"req_test_002","message":"Stage C: LLM fallback analysis completed","stage":"C_llm_fallback","analysis_result":"no","final_decision":"optional","user_prompt":"Can you explain how circuit breakers work?"}
EOF

echo "Generated structured test logs in $LOG_FILE"
echo "Log entries: $(wc -l < "$LOG_FILE")"
echo "Schema: Flat JSON structure matching logrus format"