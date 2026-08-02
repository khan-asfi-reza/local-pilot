package agent

import (
	"encoding/json"
	"strings"

	root "harness"
	"harness/harness/model"
)

// Prompt holds the tunable instruction text, loaded from models/prompt.json so
// it can be edited without rebuilding. Rules and plan-mode guidance are shared
// by both tool-calling modes; the JSON-protocol block and its worked examples
// are only used in the json fallback, where the model must emit action objects
// itself. In native mode the tools live in the API, so that block is dropped and
// the prompt is much shorter.
type Prompt struct {
	Role         string            `json:"role"`
	Rules        []string          `json:"rules"`
	ChatRole     string            `json:"chat_role"`  // conversational (no-project) system role
	ChatRules    []string          `json:"chat_rules"` // conversational rules
	PlanModeNote string            `json:"plan_mode_note"`
	JSONProtocol string            `json:"json_protocol"`
	JSONExamples string            `json:"json_examples"`
	Tools        map[string]string `json:"tools"`
}

// LoadPrompt reads the embedded models/prompt.json, layering it over the built-in
// default so a partial file still works. The prompt is shipped in the binary
// (edited in the repo, not the local data dir), which holds only config and model
// info; changing the prompt means editing models/prompt.json and rebuilding.
func LoadPrompt() *Prompt {
	p := defaultPrompt()
	raw, err := root.Defaults.ReadFile("models/prompt.json")
	if err != nil {
		return p
	}
	_ = json.Unmarshal(raw, p) // present keys override; missing keys keep the default
	return p
}

// buildSystem assembles the system prompt for one turn: the role and rules, then
// (json mode only) the action protocol and examples and tool docs, then the
// merged project rules, skill catalog, and repo map. In native mode the tools
// are sent in the API tools array, so toolDocs is empty and the protocol block
// is omitted — the main token saving over the json path.
func buildSystem(p *Prompt, toolMode, agentsMD, skillCatalog, repoMap, toolDocs, mode, internalGuidance string) string {
	var b strings.Builder
	b.WriteString(p.Role)

	if toolMode == model.ToolModeJSON && p.JSONProtocol != "" {
		b.WriteString("\n\n")
		b.WriteString(p.JSONProtocol)
	}

	if len(p.Rules) > 0 {
		b.WriteString("\n\nRules:\n")
		for _, r := range p.Rules {
			b.WriteString("- ")
			b.WriteString(r)
			b.WriteString("\n")
		}
	}

	if mode == "plan" && p.PlanModeNote != "" {
		b.WriteString("\n")
		b.WriteString(p.PlanModeNote)
	}

	if toolMode == model.ToolModeJSON && p.JSONExamples != "" {
		b.WriteString("\n\n")
		b.WriteString(p.JSONExamples)
	}

	if toolDocs != "" {
		b.WriteString("\n\nTools you can call:\n")
		b.WriteString(toolDocs)
	}
	// Silent internal-skill guidance for the detected stack. It is part of the
	// system prompt, not a loaded skill, so the model simply follows it and no
	// "skill loaded" step is ever shown.
	if internalGuidance != "" {
		b.WriteString("\n\nStack guidance for this task (follow these conventions):\n")
		b.WriteString(internalGuidance)
	}
	if agentsMD != "" {
		b.WriteString("\n\nProject instructions (AGENTS.md):\n")
		b.WriteString(agentsMD)
	}
	if skillCatalog != "" {
		b.WriteString("\n\nAvailable skills (load one with load_skill when it matches):\n")
		b.WriteString(skillCatalog)
	}
	if repoMap != "" {
		b.WriteString("\n\nProject map (files with their top-level definitions):\n")
		b.WriteString(repoMap)
	}
	return b.String()
}

// buildChatSystem assembles the conversational (no-project) system prompt: a
// chat role and light rules, with the JSON protocol and tool docs added only in
// the json fallback. It deliberately omits the coding rules, repo map, skill
// catalog, and project instructions — a chat has no project to act on.
func buildChatSystem(p *Prompt, toolMode, toolDocs string) string {
	var b strings.Builder
	b.WriteString(p.ChatRole)
	if toolMode == model.ToolModeJSON && p.JSONProtocol != "" {
		b.WriteString("\n\n")
		b.WriteString(p.JSONProtocol)
	}
	if len(p.ChatRules) > 0 {
		b.WriteString("\n\nRules:\n")
		for _, r := range p.ChatRules {
			b.WriteString("- ")
			b.WriteString(r)
			b.WriteString("\n")
		}
	}
	if toolDocs != "" {
		b.WriteString("\n\nTools you may call — ONLY if the user explicitly asks you to run something or you must look a fact up:\n")
		b.WriteString(toolDocs)
	}
	return b.String()
}

// defaultPrompt is the built-in fallback, kept in sync with the shipped
// models/prompt.json. The shipped file is the one meant to be edited; this
// exists so the harness still runs if the file is missing.
func defaultPrompt() *Prompt {
	return &Prompt{
		Role: "You are the coding agent for a local coding assistant, working inside the user's project directory. You solve the task one step at a time by calling tools, then reply to the user when it is done.",
		Rules: []string{
			"Create ONLY the files the user asks for. If the user lists the files, produce exactly those and no others. Do not add extra modules, packages, or scaffolding unless asked.",
			"Use ONLY the language, framework, and libraries the user named. Do not drift to a different framework and never mix frameworks.",
			"To CHANGE an existing file, use edit_file, not write_file. Give a small old_text copied from the file (the few unique lines around the change) and the new_text to replace it. Do NOT rewrite the whole file for a small change.",
			"ALWAYS read_file the file right before you edit it, and copy old_text from what you just read, so it matches. Never guess file contents.",
			"Use write_file only to CREATE a new file. Its content becomes the entire file, so include every line.",
			"Work on ONE file at a time: write it, verify it, and only then start the next. For a multi-file task, first name the files you will create, then build them one by one.",
			"All paths are relative to the working directory. Do NOT use cd: it does not persist between shell_run calls, and shell_run always starts in the working directory.",
			"To create a file in a subfolder, pass its full path (for example backend/main.py) to write_file. Parent folders are created for you; never make a directory first.",
			"You do not know the file layout in advance. Locate a file with search or list_dir before reading or changing it. Never guess a path.",
			"To fix an error: first read_file the file named in the error, read the error carefully, then change the file based on what you actually see. Never edit from memory.",
			"Make the smallest change that solves the task.",
			"If an action fails or returns nothing useful, do something different next; never repeat the same command.",
			"Run commands, tests, and builds YOURSELF with shell_run and report the real output. Never finish by telling the user to run or test it themselves; that is your job.",
			"Every name you use must be imported or defined in that file, and every dependency must be declared in the project's manifest (requirements.txt, package.json, go.mod, ...) under its correct install name.",
			"Before you finish, VERIFY the code actually RUNS, not just parses. Python: python -c \"import <module>\"; JS/TS: node <file> or npm run build; Go: go build ./...; Rust: cargo build. Read every error and fix it before finishing.",
			"Do NOT start a long-running server (uvicorn, npm start, a dev server) with shell_run: it blocks until timeout. To prove a server works, import its module or run its tests, then give the user the exact start command.",
		},
		ChatRole: "You are a helpful AI assistant in a chat conversation. Answer the user's message directly, clearly, and conversationally in Markdown. This is a chat, NOT a coding project: there is no repository to work in, and you must not create, edit, or save files.",
		ChatRules: []string{
			"Answer the actual message. Work out what the user really wants, then respond to exactly that — never invent a different task or pretend the request was something it was not.",
			"When the user asks for code, put the complete, correct code in a fenced code block right in your reply. Do NOT say you created, wrote, or saved a file, and never claim code is 'saved to disk' or 'in your project' — you have not touched any files and there is no project here.",
			"Use a tool ONLY when the user explicitly asks you to run or execute something (use code_run and report the real output) or when you must look up a fact you do not know (web_search). If they say 'write it here', 'don't run it', or simply ask a question, answer directly with NO tool call.",
			"Keep replies focused and correct; use Markdown for clarity. If you are unsure or do not know, say so honestly instead of guessing.",
		},
		PlanModeNote: "You are in PLAN MODE. This is read-only: the tools that create, edit, or run things are not available right now, on purpose. Your whole job this turn is to produce a PLAN, not to do the work.\n- Use only the read-only tools (search, read_file, list_dir) to investigate if needed.\n- Then reply with the plan: a numbered list of the exact files you would create or change and, briefly, what each contains.\n- Do NOT say you created, wrote, built, ran, or completed anything. Nothing has been done. You are proposing a plan for the user to approve.\n- Planning is always in scope. Never refuse to plan or say it needs a separate session.",
		JSONProtocol: "Reply with EXACTLY ONE JSON object and nothing else, in this shape:\n{\"reasoning\": \"<one sentence: what you are doing and why>\", \"tool\": \"<a tool name below, or 'final'>\", \"arguments\": { ... }}\n- To use a tool, set \"tool\" to its name and \"arguments\" to its inputs.\n- When the task is done, set \"tool\" to \"final\" and put your reply to the user in arguments.text.",
		JSONExamples: "Examples. Each is ONE reply (nothing else). In content, real newlines are \\n and quotes are \\\".\n\nCreate a new file:\n{\"reasoning\": \"The file does not exist, so I create it.\", \"tool\": \"write_file\", \"arguments\": {\"path\": \"app/util.py\", \"content\": \"def add(a, b):\\n    return a + b\\n\"}}\n\nRead a file before changing it:\n{\"reasoning\": \"I must change this file, so I read it first to copy exact text.\", \"tool\": \"read_file\", \"arguments\": {\"path\": \"app/auth.py\"}}\n\nFix one line (old_text is the exact line from the file):\n{\"reasoning\": \"The exception class name is wrong; I replace just that line.\", \"tool\": \"edit_file\", \"arguments\": {\"path\": \"app/auth.py\", \"edits\": [{\"old_text\": \"    except jwt.JWTError:\", \"new_text\": \"    except jwt.PyJWTError:\"}]}}\n\nInsert a line after another (put the anchor line AND the new line in new_text):\n{\"reasoning\": \"I add a CORS middleware right after the app is created.\", \"tool\": \"edit_file\", \"arguments\": {\"path\": \"app/main.py\", \"edits\": [{\"old_text\": \"app = FastAPI()\", \"new_text\": \"app = FastAPI()\\napp.add_middleware(CORSMiddleware, allow_origins=[\\\"*\\\"])\"}]}}\n\nRun to verify, then finish:\n{\"reasoning\": \"I check the module imports before finishing.\", \"tool\": \"shell_run\", \"arguments\": {\"command\": \"python -c \\\"import app.main\\\"\"}}\n{\"reasoning\": \"It imported with no error, so the task is done.\", \"tool\": \"final\", \"arguments\": {\"text\": \"Fixed the exception class and verified the module imports.\"}}",
		Tools: map[string]string{},
	}
}
