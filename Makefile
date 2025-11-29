 .PHONY: train compress-normal compress-dict evaluate clean-eval dict-info integration-test

SRC=Electric_Vehicle_Population_Data.csv
DICT=my_dictionary.zstd_dict
EVAL_DICT=eval.dict
EVAL_DIR=/usr/local
SERVER_ADDR=localhost:50051
SERVER_PID_FILE=/tmp/zstd-dict-server.pid

# Build the demo binary
build:
	go build -o demo ./cmd/demo

# End-to-end evaluation: train -> server -> client -> benchmark -> cleanup
evaluate: build
	@echo "=== Zstd Dictionary Compression E2E Evaluation ==="
	@echo ""
	@echo "Step 1: Training dictionary from $(EVAL_DIR)..."
	./demo train -o $(EVAL_DICT) -size 4096 $(EVAL_DIR)
	@echo ""
	@echo "Step 2: Starting server with dictionary..."
	./demo server -dict $(EVAL_DICT) & echo $$! > $(SERVER_PID_FILE)
	@sleep 2
	@echo ""
	@echo "Step 3: Testing client connections..."
	@echo "--- No compression ---"
	./demo client -path $(EVAL_DIR) -depth 2
	@echo ""
	@echo "--- Gzip compression ---"
	./demo client -path $(EVAL_DIR) -depth 2 -compress gzip
	@echo ""
	@echo "--- Zstd compression ---"
	./demo client -path $(EVAL_DIR) -depth 2 -compress zstd
	@echo ""
	@echo "--- Zstd+Dict compression ---"
	./demo client -path $(EVAL_DIR) -depth 2 -compress zstd-dict -dict $(EVAL_DICT)
	@echo ""
	@echo "Step 4: Running benchmark (20 iterations each)..."
	./demo bench -dict $(EVAL_DICT) -path $(EVAL_DIR) -depth 2 -n 20
	@echo ""
	@echo "Step 5: Running realistic scenario analysis..."
	go run ./cmd/analyze -realistic
	@echo ""
	@echo "Step 6: Cleanup..."
	@-kill $$(cat $(SERVER_PID_FILE)) 2>/dev/null || true
	@rm -f $(SERVER_PID_FILE)
	@echo ""
	@echo "=== Evaluation Complete ==="

# Clean up evaluation artifacts
clean-eval:
	@-kill $$(cat $(SERVER_PID_FILE)) 2>/dev/null || true
	@rm -f $(SERVER_PID_FILE) $(EVAL_DICT) demo

# Integration test: train dict, start server, run client tests, verify results, cleanup
integration-test: build
	@echo "=== Integration Test ==="
	@echo ""
	@# Train dictionary
	@echo "Training dictionary..."
	@./demo train -o $(EVAL_DICT) -size 4096 $(EVAL_DIR) 2>&1 | grep -v "^$$"
	@echo ""
	@# Start server in background
	@echo "Starting server..."
	@./demo server -dict $(EVAL_DICT) & echo $$! > $(SERVER_PID_FILE)
	@sleep 2
	@# Run tests for each compression method
	@echo "Testing compression methods..."
	@PASS=0; FAIL=0; \
	for method in "" "gzip" "zstd" "zstd-dict"; do \
		if [ -z "$$method" ]; then \
			name="none"; \
			./demo client -path $(EVAL_DIR) -depth 1 > /tmp/test_output.txt 2>&1; \
		elif [ "$$method" = "zstd-dict" ]; then \
			name="zstd-dict"; \
			./demo client -path $(EVAL_DIR) -depth 1 -compress $$method -dict $(EVAL_DICT) > /tmp/test_output.txt 2>&1; \
		else \
			name="$$method"; \
			./demo client -path $(EVAL_DIR) -depth 1 -compress $$method > /tmp/test_output.txt 2>&1; \
		fi; \
		if grep -q "^Files:" /tmp/test_output.txt; then \
			echo "  [PASS] $$name"; \
			PASS=$$((PASS + 1)); \
		else \
			echo "  [FAIL] $$name"; \
			cat /tmp/test_output.txt; \
			FAIL=$$((FAIL + 1)); \
		fi; \
	done; \
	echo ""; \
	echo "Results: $$PASS passed, $$FAIL failed"; \
	rm -f /tmp/test_output.txt; \
	kill $$(cat $(SERVER_PID_FILE)) 2>/dev/null || true; \
	rm -f $(SERVER_PID_FILE); \
	if [ $$FAIL -gt 0 ]; then exit 1; fi
	@echo ""
	@echo "=== Integration Test Passed ==="

# Display dictionary information
dict-info:
	@if [ -f "$(EVAL_DICT)" ]; then \
		echo "=== Dictionary Info ==="; \
		echo "File: $(EVAL_DICT)"; \
		file $(EVAL_DICT); \
		ls -lh $(EVAL_DICT); \
		echo ""; \
		echo "=== Embedded Strings (sample) ==="; \
		strings $(EVAL_DICT) | head -30; \
		echo "..."; \
		echo ""; \
		echo "=== Total unique strings ==="; \
		strings $(EVAL_DICT) | wc -l | xargs echo "Count:"; \
	else \
		echo "Dictionary not found: $(EVAL_DICT)"; \
		echo "Run 'make evaluate' first to generate it."; \
	fi

# Original targets from initial experimentation
train:
	@#zstd --train -o my_dictionary.zstd_dict -B1024 --maxdict=10240 training_file1.txt training_file2.log Electric_Vehicle_Population_Data.csv
	echo training on ${SRC}
	zstd --train -o ${DICT} -B1024 --maxdict=10240 training_file1.txt training_file2.log ${SRC}

compress-normal:
	@#zstd -k -o normal_compression.csv.zst  Electric_Vehicle_Population_Data.csv
	echo compressing ${SRC}
	@zstd -k -o normal_compression.csv.zst  $(SRC)

compress-dict:
	@#zstd -k -o normal_compression.csv.zst  Electric_Vehicle_Population_Data.csv
	echo compressing ${SRC}
	@zstd -k -D ${DICT} -o dict_compression.csv.zst  $(SRC)
