# Example SKILL.md Files

This directory contains example SKILL.md files demonstrating various skill configurations for the Baaaht Orchestrator skills system.

## SKILL.md Format

Each SKILL.md file uses JSON frontmatter to define skill metadata, followed by markdown content that provides additional documentation.

### Required Fields

- `name` - Internal identifier for the skill (e.g., "git-helper", "code-reviewer")
- `display_name` - Human-readable name (e.g., "Git Helper", "Code Reviewer")
- `description` - Short description of what the skill does

### Optional Fields

- `version` - Skill version (default: "1.0.0")
- `author` - Skill author name
- `agent_types` - Array of agent types that can use this skill (default: ["all"])
  - "assistant" - Only assistant agents
  - "worker" - Only worker agents
  - "all" - All agent types
- `capabilities` - Array of capability objects
  - `type` - Capability type: "tool", "prompt", or "behavior"
  - `name` - Capability name
  - `description` - Capability description
  - `config` - Optional configuration specific to the capability
- `tags` - Array of tags for categorization
- `category` - Skill category (e.g., "development", "productivity", "analysis")
- `keywords` - Array of search keywords
- `required_apis` - APIs required by this skill
- `dependencies` - Other skills this depends on
- `labels` - Key-value pairs for additional metadata
- `verified` - Whether user has verified this skill (default: false)

## Example 1: Basic Skill (Minimal)

```json
{
  "name": "hello-world",
  "display_name": "Hello World",
  "description": "A simple skill that demonstrates the basic SKILL.md format"
}
```

# Hello World

A simple skill that demonstrates the basic SKILL.md format with minimal required fields.

This skill can be used by all agent types and provides a friendly greeting capability.

## Example 2: Tool Capability Skill

```json
{
  "name": "git-helper",
  "display_name": "Git Helper",
  "description": "Provides Git-related tools and commands for version control operations",
  "version": "1.2.0",
  "author": "Baaaht Team",
  "agent_types": ["all"],
  "capabilities": [
    {
      "type": "tool",
      "name": "git_status",
      "description": "Check the status of the Git repository",
      "config": {
        "command": "git status",
        "timeout": "30s"
      }
    },
    {
      "type": "tool",
      "name": "git_commit",
      "description": "Create a new commit with staged changes",
      "config": {
        "command": "git commit",
        "requires_message": true
      }
    },
    {
      "type": "tool",
      "name": "git_push",
      "description": "Push commits to a remote repository",
      "config": {
        "command": "git push",
        "confirmation_required": true
      }
    }
  ],
  "tags": ["git", "version-control", "development"],
  "category": "development",
  "keywords": ["git", "version control", "vcs", "commit", "push", "pull"]
}
```

# Git Helper

Provides Git-related tools and commands for version control operations. This skill enables agents to interact with Git repositories for common version control tasks.

## Capabilities

### git_status
Check the status of the Git repository to see modified, staged, and untracked files.

### git_commit
Create a new commit with staged changes. Requires a commit message to be provided.

### git_push
Push commits to a remote repository. Requires confirmation before executing.

## Usage

When an agent needs to perform Git operations, this skill provides the necessary tools to:
- Check repository status
- Create commits
- Push changes to remote

## Example 3: Prompt Capability Skill

```json
{
  "name": "code-reviewer",
  "display_name": "Code Reviewer",
  "description": "Enhances code review capabilities with specialized prompts and analysis tools",
  "version": "2.0.0",
  "author": "Baaaht Team",
  "agent_types": ["assistant"],
  "capabilities": [
    {
      "type": "prompt",
      "name": "review_style",
      "description": "Prompt template for reviewing code style and formatting",
      "config": {
        "template": "Review the following code for style and formatting issues according to {style_guide} standards:\n\n{code}\n\nProvide specific suggestions for improvement."
      }
    },
    {
      "type": "prompt",
      "name": "review_security",
      "description": "Prompt template for security code review",
      "config": {
        "template": "Perform a security review of the following code:\n\n{code}\n\nIdentify potential security vulnerabilities including:\n- Injection attacks\n- Authentication/authorization issues\n- Data exposure\n- Cryptographic weaknesses"
      }
    },
    {
      "type": "tool",
      "name": "analyze_complexity",
      "description": "Analyze code complexity metrics",
      "config": {
        "metrics": ["cyclomatic", "cognitive", "maintainability"]
      }
    }
  ],
  "tags": ["code-review", "analysis", "quality"],
  "category": "development",
  "keywords": ["review", "code quality", "security", "complexity"]
}
```

# Code Reviewer

Enhances code review capabilities with specialized prompts and analysis tools for assistant agents.

## Capabilities

### Review Prompts
- **review_style**: Reviews code for style and formatting issues
- **review_security**: Performs security analysis to identify vulnerabilities

### Analysis Tools
- **analyze_complexity**: Calculates cyclomatic, cognitive, and maintainability metrics

## Usage

This skill provides specialized prompts and tools for conducting thorough code reviews:
1. Style reviews ensure code follows established standards
2. Security reviews identify potential vulnerabilities
3. Complexity metrics help identify overly complex code

## Example 4: Behavior Capability Skill

```json
{
  "name": "concise-mode",
  "display_name": "Concise Mode",
  "description": "Modifies agent behavior to provide shorter, more focused responses",
  "version": "1.0.0",
  "agent_types": ["assistant"],
  "capabilities": [
    {
      "type": "behavior",
      "name": "concise_responses",
      "description": "Enforces concise response style",
      "config": {
        "max_sentences": 5,
        "prefer_bullets": true,
        "omit_preamble": true
      }
    }
  ],
  "tags": ["behavior", "productivity", "ux"],
  "category": "productivity"
}
```

# Concise Mode

Modifies agent behavior to provide shorter, more focused responses, improving efficiency for users who prefer brevity.

## Behavior Modification

When active, this skill configures the assistant to:
- Limit responses to 5 sentences maximum
- Use bullet points when listing items
- Omit conversational preambles

## Example 5: Full-Featured Skill

```json
{
  "name": "docker-manager",
  "display_name": "Docker Manager",
  "description": "Comprehensive Docker container and image management capabilities",
  "version": "3.1.0",
  "author": "Baaaht Team",
  "agent_types": ["worker"],
  "capabilities": [
    {
      "type": "tool",
      "name": "list_containers",
      "description": "List all Docker containers with their status",
      "config": {
        "show_all": true,
        "format": "table"
      }
    },
    {
      "type": "tool",
      "name": "inspect_container",
      "description": "Get detailed information about a container",
      "config": {
        "output_format": "json"
      }
    },
    {
      "type": "tool",
      "name": "container_logs",
      "description": "Fetch logs from a container",
      "config": {
        "tail": 100,
        "follow": false
      }
    },
    {
      "type": "prompt",
      "name": "dockerfile_analyzer",
      "description": "Analyze Dockerfile for best practices",
      "config": {
        "check_layers": true,
        "check_security": true,
        "suggest_optimizations": true
      }
    }
  ],
  "tags": ["docker", "containers", "devops"],
  "category": "development",
  "keywords": ["docker", "container", "image", "devops", "containers"],
  "required_apis": ["docker.sock"],
  "dependencies": ["bash-helper"],
  "labels": {
    "language": "go",
    "platform": "linux",
    "privileged": "true"
  }
}
```

# Docker Manager

Comprehensive Docker container and image management capabilities for worker agents.

## Capabilities

### Container Management Tools
- **list_containers**: List all containers with status information
- **inspect_container**: Get detailed container configuration and state
- **container_logs**: Fetch and display container logs

### Image Analysis
- **dockerfile_analyzer**: Analyze Dockerfiles for best practices, security, and optimization opportunities

## Requirements

- Docker socket access (`docker.sock`)
- Bash Helper skill dependency
- Linux platform support

## Permissions

This skill requires privileged access to interact with the Docker daemon.

## Example 6: Multi-Agent Skill

```json
{
  "name": "team-collaboration",
  "display_name": "Team Collaboration",
  "description": "Enables collaboration features across multiple agents and users",
  "version": "1.5.0",
  "author": "Baaaht Team",
  "agent_types": ["assistant", "worker"],
  "capabilities": [
    {
      "type": "tool",
      "name": "share_context",
      "description": "Share context between agents in the same session",
      "config": {
        "scope": "session",
        "include_metadata": true
      }
    },
    {
      "type": "prompt",
      "name": "status_update",
      "description": "Generate standardized status updates for team communication",
      "config": {
        "template": "## Status Update\n\n**Agent**: {agent_name}\n**Task**: {task}\n**Status**: {status}\n**Progress**: {progress}%\n\n{details}"
      }
    },
    {
      "type": "behavior",
      "name": "notify_changes",
      "description": "Automatically notify team members of relevant changes",
      "config": {
        "events": ["task_complete", "error", "blocker"],
        "channels": ["slack", "email"]
      }
    }
  ],
  "tags": ["collaboration", "team", "communication"],
  "category": "productivity",
  "keywords": ["team", "collaboration", "sharing", "notification"],
  "labels": {
    "enterprise": "true",
    "requires_auth": "true"
  }
}
```

# Team Collaboration

Enables collaboration features across multiple agents and users within enterprise environments.

## Features

### Context Sharing
Share context between agents operating in the same session for improved coordination.

### Communication
- Generate standardized status updates for team communication
- Automatic notifications for important events

## Events

This skill monitors and notifies on:
- Task completion
- Error occurrences
- Blockers detected

## Notes

- Requires authentication for enterprise features
- Supports Slack and email notification channels

---

## File Location

Skills should be placed in the following directory structure:

```
skills/
├── user/
│   └── {user_id}/
│       └── {skill_name}/
│           └── SKILL.md
└── group/
    └── {group_id}/
        └── {skill_name}/
            └── SKILL.md
```

## Testing Skills

To test a skill, place the SKILL.md file in the appropriate directory and use the skills API:

```bash
# List all skills
curl http://localhost:8080/api/v1/skills

# Get skill details
curl http://localhost:8080/api/v1/skills/{skill_id}

# Activate a skill
curl -X PUT http://localhost:8080/api/v1/skills/{skill_id}/activate
```
