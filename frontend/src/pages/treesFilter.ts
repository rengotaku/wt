import type { TreeItem } from "@/api/trees";

export function filterTrees(
  trees: TreeItem[],
  filterText: string,
  showMain: boolean,
  repoFilter: string,
): TreeItem[] {
  return trees.filter((t) => {
    if (!showMain && t.is_main) return false;
    if (repoFilter && t.repo !== repoFilter) return false;
    if (filterText) {
      const q = filterText.toLowerCase();
      if (
        !t.wt_name.toLowerCase().includes(q) &&
        !t.repo.toLowerCase().includes(q) &&
        !t.branch.toLowerCase().includes(q)
      )
        return false;
    }
    return true;
  });
}
