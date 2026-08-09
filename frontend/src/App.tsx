import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Toaster } from "sonner";
import { Layout } from "@/components";
import { TreesPage, ReposPage, MaintenancePage, SettingsPage, NotFoundPage } from "@/pages";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 30,
      retry: 1,
    },
  },
});

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Toaster richColors position="bottom-right" />
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<TreesPage />} />
            <Route path="repos" element={<ReposPage />} />
            <Route path="maintenance" element={<MaintenancePage />} />
            {/* 旧 /gc /ports のブックマーク・履歴からのアクセスを維持する */}
            <Route path="gc" element={<Navigate to="/maintenance" replace />} />
            <Route path="ports" element={<Navigate to="/maintenance" replace />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
