import { describe, it, expect } from "vitest";
import { filterTrees } from "./treesFilter";
import type { TreeItem, IssueDetail } from "@/api/trees";

const makeTree = (overrides: Partial<TreeItem>): TreeItem => ({
  wt_name: "repo--feat-issue-1-abc",
  repo: "repo",
  label: "[feat] feat/issue-1-abc (2024-01-01)",
  path: "/home/user/Workspace/repo/repo--feat-issue-1-abc",
  created_at: "2024-01-01",
  diff_count: 0,
  is_main: false,
  branch: "feat/issue-1-abc",
  pinned: false,
  auto_start: false,
  ...overrides,
});

describe("filterTrees", () => {
  describe("showMain", () => {
    it("hides main worktrees when showMain is false", () => {
      const trees = [
        makeTree({ is_main: true }),
        makeTree({ is_main: false, wt_name: "repo--feat" }),
      ];
      expect(filterTrees(trees, "", false, "")).toHaveLength(1);
    });

    it("shows main worktrees when showMain is true", () => {
      const trees = [makeTree({ is_main: true }), makeTree({ is_main: false })];
      expect(filterTrees(trees, "", true, "")).toHaveLength(2);
    });
  });

  describe("repoFilter", () => {
    it("keeps only matching repo when repoFilter is set", () => {
      const trees = [makeTree({ repo: "alpha" }), makeTree({ repo: "beta" })];
      expect(filterTrees(trees, "", true, "alpha")).toHaveLength(1);
    });

    it("shows all repos when repoFilter is empty", () => {
      const trees = [makeTree({ repo: "alpha" }), makeTree({ repo: "beta" })];
      expect(filterTrees(trees, "", true, "")).toHaveLength(2);
    });
  });

  describe("filterText – wt_name", () => {
    it("matches by wt_name substring", () => {
      const trees = [
        makeTree({
          wt_name: "myrepo--feat-issue-28-d591e886",
          branch: "feat/issue-28-d591e886",
        }),
        makeTree({ wt_name: "myrepo--fix-bug", branch: "fix/bug" }),
      ];
      expect(filterTrees(trees, "feat-issue-28", true, "")).toHaveLength(1);
    });

    it("is case-insensitive for wt_name", () => {
      const trees = [makeTree({ wt_name: "Repo--Feat-Issue-1-abc" })];
      expect(filterTrees(trees, "feat-issue", true, "")).toHaveLength(1);
    });
  });

  describe("filterText – repo", () => {
    it("matches by repo substring", () => {
      const trees = [makeTree({ repo: "my-project" }), makeTree({ repo: "other-app" })];
      expect(filterTrees(trees, "my-project", true, "")).toHaveLength(1);
    });
  });

  describe("filterText – branch (new feature)", () => {
    it("matches when filterText equals the branch name with slash", () => {
      const trees = [
        makeTree({
          wt_name: "myrepo--feat-issue-28-d591e886",
          branch: "feat/issue-28-d591e886",
        }),
        makeTree({ wt_name: "myrepo--fix-bug", branch: "fix/bug" }),
      ];
      expect(filterTrees(trees, "feat/issue-28-d591e886", true, "")).toHaveLength(1);
    });

    it("matches partial branch name with slash", () => {
      const trees = [
        makeTree({
          wt_name: "myrepo--feat-issue-28-d591e886",
          branch: "feat/issue-28-d591e886",
        }),
        makeTree({ wt_name: "myrepo--fix-bug", branch: "fix/bug" }),
      ];
      expect(filterTrees(trees, "feat/issue-28", true, "")).toHaveLength(1);
    });

    it("is case-insensitive for branch", () => {
      const trees = [makeTree({ branch: "feat/Issue-28-D591E886" })];
      expect(filterTrees(trees, "feat/issue-28", true, "")).toHaveLength(1);
    });

    it("does not match other worktrees when branch name is given", () => {
      const trees = [
        makeTree({
          wt_name: "myrepo--feat-issue-28-d591e886",
          branch: "feat/issue-28-d591e886",
        }),
        makeTree({ wt_name: "myrepo--feat-issue-99-zzz", branch: "feat/issue-99-zzz" }),
      ];
      expect(filterTrees(trees, "feat/issue-28-d591e886", true, "")).toHaveLength(1);
    });
  });

  describe("filterText – parent issue", () => {
    const makeIssueDataMap = (
      overrides: IssueDetail[]
    ): Record<string, IssueDetail[]> => ({
      repo: overrides,
    });

    it("matches by parent issue number with # prefix", () => {
      const trees = [
        makeTree({ issue: "#84" }),
        makeTree({
          issue: "#85",
          wt_name: "repo--feat-issue-85-xxx",
          branch: "feat/issue-85-xxx",
        }),
      ];
      const issueDataMap = makeIssueDataMap([
        {
          number: 84,
          state: "OPEN",
          parent_number: 70,
          parent_url: "https://github.com/owner/repo/issues/70",
        },
        {
          number: 85,
          state: "OPEN",
          parent_number: 70,
          parent_url: "https://github.com/owner/repo/issues/70",
        },
      ]);
      expect(filterTrees(trees, "#70", true, "", issueDataMap)).toHaveLength(2);
    });

    it("does not match when parent issue differs", () => {
      const trees = [
        makeTree({ issue: "#84" }),
        makeTree({
          issue: "#85",
          wt_name: "repo--feat-issue-85-xxx",
          branch: "feat/issue-85-xxx",
        }),
      ];
      const issueDataMap = makeIssueDataMap([
        { number: 84, state: "OPEN", parent_number: 70, parent_url: "" },
        { number: 85, state: "OPEN", parent_number: 71, parent_url: "" },
      ]);
      expect(filterTrees(trees, "#70", true, "", issueDataMap)).toHaveLength(1);
    });

    it("does not match when issue has no parent", () => {
      const trees = [makeTree({ issue: "#84" })];
      const issueDataMap = makeIssueDataMap([{ number: 84, state: "OPEN" }]);
      expect(filterTrees(trees, "#70", true, "", issueDataMap)).toHaveLength(0);
    });

    it("works without issueDataMap (backward compat)", () => {
      const trees = [makeTree({ issue: "#84" })];
      expect(filterTrees(trees, "#70", true, "")).toHaveLength(0);
    });
  });

  describe("no filterText", () => {
    it("returns all trees when filterText is empty", () => {
      const trees = [makeTree({}), makeTree({ wt_name: "repo--other" })];
      expect(filterTrees(trees, "", true, "")).toHaveLength(2);
    });
  });
});
