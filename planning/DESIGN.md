
Note: This file is human thought input only.

# Promptcellar for Claude Code

Promptcellar is an open source standard to track agentic coding prompts. Tracking prompts allows teams to track the pure human thinking and signal that was used to build any given software. Being able to track this unlocks many new use cases around collaboration, software maintenance, quality control, etc.

Promptcellar uses a simple and decentralized mechanism to store prompts:

- Using an MCP plugin (or tighter integration) any prompt is captured from Claude Code
- These prompts are stored in a project's folder under .prompts/ in a retrieval friendly data structure
- Prompts that include passwords, secrets, access tokens, etc. are excluded from the MCP tooling
- The data captured in .prompts/ is rich and can be read by open source tools and proprietary dashboards

## Ideas

- We want the format used for data storage to be an open standard "Promptcellar Logging Format" described 
- Each stored prompt should include author (Github mail/user), the version of the format (e.g. plf-1), the input prompt, the tokens used in the session, an identifier of the context window/session, maybe information/references to previous context present, the github context such as last commit and branch.
- We want to make sure no merge conflicts happen if prompts come from other branches
- Directory structure should include date/hour/etc so that chronological loading is possible


