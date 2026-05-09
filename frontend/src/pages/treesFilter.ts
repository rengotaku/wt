import type { TreeItem, IssueDetail } from "@/api/trees";

export function filterTrees(
  trees: TreeItem[],
  filterText: string,
  showMain: boolean,
  repoFilter: string,
  issueDataMap?: Record<string, IssueDetail[]>,
): TreeItem[] {
  return trees.filter((t) => {
    if (!showMain && t.is_main) return false;
    if (repoFilter && t.repo !== repoFilter) return false;
    if (filterText) {
      const q = filterText.toLowerCase();
      let parentIssueStr = "";
      if (issueDataMap && t.issue) {
        const num = parseInt(t.issue.replace("#", ""), 10);
        const detail = issueDataMap[t.repo]?.find((d) => d.number === num);
        if (detail?.parent_number) {
          parentIssueStr = `#${detail.parent_number}`;
        }
      }
      if (
        !t.wt_name.toLowerCase().includes(q) &&
        !t.repo.toLowerCase().includes(q) &&
        !t.branch.toLowerCase().includes(q) &&
        !parentIssueStr.includes(q)
      )
        return false;
    }
    return true;
  });
}
