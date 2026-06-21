import { useState, useEffect } from "react";

// Tailwind の md ブレークポイント(768px)未満を「モバイル」とみなす。
const MOBILE_QUERY = "(max-width: 767.98px)";

function getMatch(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    // matchMedia 非対応環境（テストの既定モック含む）はデスクトップ扱いにする。
    return false;
  }
  return window.matchMedia(MOBILE_QUERY).matches;
}

/**
 * ビューポート幅が Tailwind の md(768px) 未満かどうかを返すフック。
 *
 * テーブル⇄カードのように DOM 構造そのものを切り替える分岐に使う。CSS の
 * `hidden`/`md:block` で両方を同時に DOM へ出すと、同じ aria-label を持つ要素が
 * 二重に存在して Testing Library のクエリが多重マッチで壊れる。これを避けるため、
 * 片方だけを描画する JS 分岐に matchMedia を用いる。初期値も matchMedia から同期
 * 取得し、初回描画でのちらつき（テーブル⇄カードの一瞬の入れ替わり）を防ぐ。
 */
export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState<boolean>(getMatch);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const mql = window.matchMedia(MOBILE_QUERY);
    const onChange = () => setIsMobile(mql.matches);
    // マウント後に最新状態へ同期（初期値取得と描画の間に幅が変わった場合に追従）。
    onChange();
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return isMobile;
}
