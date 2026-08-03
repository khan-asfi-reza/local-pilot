import { useEffect, useState } from 'react';
import { Routes, Route } from 'react-router-dom';
import { Home } from './features/home/Home';
import { ChatWindow } from './components/chat/ChatWindow';
import { Builder } from './features/builder/Builder';
import { CodePage } from './features/code/CodePage';
import { SettingsPage } from './features/settings/SettingsPage';
import { Onboarding } from './features/onboarding/Onboarding';
import { profile as profileApi } from './lib/api';

export default function App() {
  const [needsOnboarding, setNeedsOnboarding] = useState(false);

  useEffect(() => {
    profileApi.get().then((p) => {
      if (p && !p.onboarded) setNeedsOnboarding(true);
    });
  }, []);

  return (
    <div className="h-full font-sans antialiased">
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/chat" element={<ChatWindow />} />
        {/* The open project's id lives in the URL so reload/back restores it. */}
        <Route path="/builder" element={<Builder />} />
        <Route path="/builder/:projectId" element={<Builder />} />
        <Route path="/code" element={<CodePage />} />
        <Route path="/code/:projectId" element={<CodePage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Routes>

      {needsOnboarding && <Onboarding onDone={() => setNeedsOnboarding(false)} />}
    </div>
  );
}
