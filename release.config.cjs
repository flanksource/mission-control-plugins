const plugin = process.env.PLUGIN;

if (!plugin) {
  throw new Error("PLUGIN environment variable is required");
}

module.exports = {
  branches: ["main"],
  tagFormat: `${plugin}/v\${version}`,
  plugins: [
    [
      "@semantic-release/commit-analyzer",
      {
        preset: "conventionalcommits",
        releaseRules: [
          { breaking: true, scope: plugin, release: "major" },
          { type: "feat", scope: plugin, release: "minor" },
          { type: "fix", scope: plugin, release: "patch" },
          { type: "perf", scope: plugin, release: "patch" },
          { type: "revert", scope: plugin, release: "patch" },
          { scope: "*", release: false },
          { type: "*", release: false },
        ],
      },
    ],
    [
      "@semantic-release/release-notes-generator",
      {
        preset: "conventionalcommits",
        writerOpts: {
          transform: (commit) => (commit.scope === plugin ? commit : false),
        },
      },
    ],
    [
      "@semantic-release/github",
      {
        successComment: false,
        failComment: false,
      },
    ],
  ],
};
