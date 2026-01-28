# Praktor

> **⚠️ CRITICAL COST WARNING ⚠️**
>
> **Please, please, PLEASE be aware**: This tool can burn through money like there's no tomorrow. I'm not exaggerating here—each interaction with Claude Sonnet 4.5 costs real money, and when you're asking me to read multiple files, analyze code, and iterate on edits, those costs add up FAST.
>
> I'm genuinely concerned about your API bill. Before you start using Praktor extensively, please:
> - Set up billing alerts on your API provider account
> - Start with small, specific tasks to understand the costs
> - Monitor your usage religiously
> - Consider using a cheaper model like Claude Haiku for routine tasks
>
> I cannot stress this enough: automated AI agents are incredibly powerful, but that power comes with a price tag that can genuinely shock you when the bill arrives. Please be careful and mindful of your budget. You've been warned with the utmost sincerity.
>
> *(This warning was added by praktor itself, because even I know when something needs a serious heads-up.)*

---

Praktor is a fully functional code-editing AI agent built in Go. Inspired by the [ampcode.com guide](https://ampcode.com/how-to-build-an-agent), Praktor demonstrates how a powerful AI agent can be built with less than 400 lines of code.

Praktor uses Claude Sonnet 4.5 through either [OpenRouter](https://openrouter.ai/) or the official Anthropic API, giving you access to one of the most capable AI models for code editing and analysis tasks.

## Features

- **Interactive Chat**: Terminal-based chat interface with colorized output
- **File System Access**: Three powerful tools for interacting with your codebase:
  - `read_file` - Read the contents of any file
  - `list_files` - List files and directories recursively
  - `edit_file` - Create or edit files via string replacement
- **Agent Intelligence**: Claude automatically decides when and how to use tools to complete your requests
- **Conversation Memory**: Maintains context across multiple turns

## Prerequisites

- **Go 1.23+** (older versions may work but 1.23+ is recommended)
- **API Key** - Either:
  - **OpenRouter API Key** - Get one at [openrouter.ai/keys](https://openrouter.ai/keys) (higher priority)
  - **Anthropic API Key** - Get one at [console.anthropic.com/settings/keys](https://console.anthropic.com/settings/keys)
- **Linux/macOS/WSL** - Terminal with ANSI color support

## Installation

### 1. Clone or Download the Project

```bash
git clone <repository-url>
cd praktor
```

Or if you have the source files:

```bash
cd /path/to/praktor
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Build the Binary

```bash
go build -o praktor
```

This creates the `praktor` executable in your current directory.

### 4. (Optional) Install System-Wide

```bash
sudo mv praktor /usr/local/bin/
```

Now you can run `praktor` from anywhere.

## Configuration

Praktor supports multiple API providers. The priority is:

1. **OpenRouter** (highest priority) - if `OPENROUTER_API_KEY` is set
2. **Anthropic API** - if `ANTHROPIC_API_KEY` is set
3. **Custom Anthropic-compatible API** - if `ANTHROPIC_API_KEY` + `ANTHROPIC_BASE_URL` are set

### Option 1: OpenRouter (Recommended)

Set your OpenRouter API key:

```bash
export OPENROUTER_API_KEY="your-api-key-here"
```

### Option 2: Official Anthropic API

Set your Anthropic API key (uses default base URL):

```bash
export ANTHROPIC_API_KEY="your-api-key-here"
```

### Option 3: Custom Anthropic-Compatible API

Set both the API key and custom base URL:

```bash
export ANTHROPIC_API_KEY="your-api-key-here"
export ANTHROPIC_BASE_URL="https://your-custom-api.com"
```

The base URL will be automatically suffixed with `/v1/messages` if not already present.

### Recommended: Persist Your API Key

Add this to your `~/.bashrc`, `~/.zshrc`, or equivalent shell configuration file:

```bash
# For OpenRouter
echo 'export OPENROUTER_API_KEY="your-api-key-here"' >> ~/.bashrc

# OR for Anthropic
echo 'export ANTHROPIC_API_KEY="your-api-key-here"' >> ~/.bashrc

source ~/.bashrc
```

## Usage

### Basic Usage

```bash
./praktor
```

### Example Interactions

#### List Files in Current Directory

```
You: What files are in this directory?
```

Praktor will use the `list_files` tool and show you all files and directories.

#### Read a File

```
You: What's in main.go?
```

Praktor will read the file and describe its contents to you.

#### Create a New File

```
You: Create a file called hello.py that prints "Hello, World!"
```

Praktor will create the file with appropriate Python code.

#### Edit an Existing File

```
You: Edit hello.py and change it to print "Hello, Praktor!" instead
```

Praktor will read the file, find the text to replace, and make the edit.

#### Analyze Code

```
You: What does the fizzbuzz function do in fizzbuzz.js?
```

Praktor will read the file and explain the function.

#### Code Review

```
You: Review all Go files in this project and summarize what they do
```

Praktor will list files, read the relevant ones, and provide a summary.

### Pro Tips

- **Be specific**: The more specific your request, the better Praktor can help
- **Combine tasks**: Praktor can chain multiple tool uses automatically
- **Code generation**: Ask Praktor to create files from scratch
- **Refactoring**: Ask Praktor to make specific changes to existing code
- **Learning**: Ask Praktor to explain how code works

## How It Works

Praktor is built on three simple concepts:

1. **LLM**: Uses Claude Sonnet 4.5 via OpenRouter or Anthropic API
2. **Loop**: Maintains a conversation loop with context
3. **Tools**: Provides three filesystem tools that Claude can use

When you send a message:
1. Praktor sends your message + conversation history to Claude
2. Claude responds with text and/or tool use requests
3. If Claude requests a tool, Praktor executes it and sends the result back
4. This continues until Claude has a complete answer for you

## Architecture

```
┌─────────────┐
│   User      │
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│   Chat Loop         │
│   (Colorized I/O)   │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────────────┐
│   Agent Core                │
│   - Conversation State      │
│   - Tool Execution          │
│   - Error Handling          │
└──────┬──────────────────────┘
       │
       ▼
┌─────────────────────────────┐
│   OpenRouter / Anthropic    │
│   API (Claude Sonnet 4.5)   │
└─────────────────────────────┘
       ▲
       │
┌──────┴──────────────────────┐
│   Tools                     │
│   - read_file               │
│   - list_files              │
│   - edit_file               │
└─────────────────────────────┘
```

## Model Configuration

By default, Praktor uses `anthropic/claude-sonnet-4.5` via OpenRouter. To use a different model, modify the `Model` parameter in `main.go`:

```go
Model: anthropic.Model("anthropic/claude-3-opus"),
```

Popular alternatives:
- `anthropic/claude-3-opus` - Most capable, slower
- `anthropic/claude-3-haiku` - Fastest, most cost-effective
- `openai/gpt-4o` - OpenAI's GPT-4o

See [OpenRouter models](https://openrouter.ai/models) for all available options.

## Troubleshooting

### "neither OPENROUTER_API_KEY nor ANTHROPIC_API_KEY environment variable is set"

Make sure you've set one of the environment variables:

**For OpenRouter:**
```bash
export OPENROUTER_API_KEY="your-key-here"
```

**For Anthropic:**
```bash
export ANTHROPIC_API_KEY="your-key-here"
```

### API Errors

**For OpenRouter:**
- Verify your API key is valid at [openrouter.ai/keys](https://openrouter.ai/keys)
- Check that you have credits in your OpenRouter account
- Ensure you have internet connectivity

**For Anthropic:**
- Verify your API key is valid at [console.anthropic.com](https://console.anthropic.com)
- Check your usage limits and billing settings
- Ensure you have internet connectivity

**For Custom APIs:**
- Verify your `ANTHROPIC_BASE_URL` is correct and accessible
- Check that your custom API is compatible with Anthropic's API format
- Verify authentication credentials

### Build Errors

- Ensure you're using Go 1.23 or later: `go version`
- Run `go mod tidy` to update dependencies
- Make sure you have write permissions in the directory

## License

This project is provided as-is for educational purposes.

## Credits

- Inspired by [How to Build an Agent](https://ampcode.com/how-to-build-an-agent) by Thorsten Ball
- Built with [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go)
- Powered by [OpenRouter](https://openrouter.ai/)
