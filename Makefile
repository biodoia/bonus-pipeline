.PHONY: all build proto daemon tui agent legacy clean start stop

PROTO_OUT = proto/pipelinepb
SESSION   = bonus-pipeline

all: build

# ── Build ────────────────────────────────────────────────────────────────────

build:
	@mkdir -p bin
	go build -o bin/daemon   ./cmd/daemon
	go build -o bin/tui      ./cmd/tui
	go build -o bin/pipeline ./

proto:
	@mkdir -p $(PROTO_OUT)
	protoc --go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
	       --go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
	       proto/pipeline.proto

# ── Run individual ───────────────────────────────────────────────────────────

daemon:
	go run ./cmd/daemon -data .

tui:
	go run ./cmd/tui -data .

agent:
	cd agent && python main.py --daemon localhost:50051

agent-demo:
	cd agent && python main.py --demo --ollama http://localhost:11434

legacy:
	go run . $(ARGS)

# ── Tmux orchestration ──────────────────────────────────────────────────────

start: build
	@if tmux has-session -t $(SESSION) 2>/dev/null; then \
		echo "Session '$(SESSION)' already running. Use 'make stop' first or 'tmux attach -t $(SESSION)'"; \
		exit 1; \
	fi
	@echo "Starting bonus-pipeline in tmux session '$(SESSION)'..."
	@tmux new-session -d -s $(SESSION) -n main
	@# Pane 0: daemon
	@tmux send-keys -t $(SESSION):main "cd $(CURDIR) && ./bin/daemon -data $(CURDIR)" Enter
	@sleep 1
	@# Pane 1: TUI (vertical split)
	@tmux split-window -h -t $(SESSION):main
	@tmux send-keys -t $(SESSION):main.1 "cd $(CURDIR) && ./bin/tui -data $(CURDIR) -addr localhost:50051" Enter
	@# Pane 2: agent (horizontal split on right pane)
	@tmux split-window -v -t $(SESSION):main.1
	@if command -v ollama >/dev/null 2>&1 && curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then \
		tmux send-keys -t $(SESSION):main.2 "cd $(CURDIR)/agent && python main.py --daemon localhost:50051" Enter; \
	else \
		tmux send-keys -t $(SESSION):main.2 "echo 'Agent: Ollama non disponibile. Avvia con: ollama serve && make agent'" Enter; \
	fi
	@# Layout: daemon left, TUI top-right, agent bottom-right
	@tmux select-pane -t $(SESSION):main.1
	@echo ""
	@echo "bonus-pipeline running in tmux session '$(SESSION)'"
	@echo ""
	@echo "  tmux attach -t $(SESSION)    # attach alla sessione"
	@echo "  make stop                    # ferma tutto"
	@echo ""
	@echo "Layout:"
	@echo "  [0] daemon    │ [1] TUI"
	@echo "                │ [2] agent"
	@echo ""

stop:
	@if tmux has-session -t $(SESSION) 2>/dev/null; then \
		tmux kill-session -t $(SESSION); \
		echo "Session '$(SESSION)' stopped."; \
	else \
		echo "No session '$(SESSION)' running."; \
	fi

# ── Cleanup ──────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/ daemon tui pipeline
