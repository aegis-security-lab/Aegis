// Package capability resolves declarative execution capabilities into concrete
// agentcore tools, instructions, and lifecycle resources.
//
// Coordination selects capability references. Agent runtimes use Registry to
// materialize them, keeping provider SDKs, Phone clients, MCP processes, and
// Skill loaders out of scheduling code.
package capability
