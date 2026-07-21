import { Suspense, lazy } from 'react';
import { Routes, Route } from 'react-router-dom';
import { Home } from './features/home/Home';
import { ChatWindow } from './components/chat/ChatWindow';
import { Builder } from './features/builder/Builder';
import { Loader } from './components/chat/Loader';

// The Code IDE lives in features/code/, built as a separate part. Load it
// optionally via import.meta.glob so the app builds and runs even before that
// file exists; once it lands, Vite bundles it normally.
const codeModules = import.meta.glob('./features/code/CodePage.jsx');
const codeLoader = codeModules['./features/code/CodePage.jsx'];

function CodeUnavailable() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-zinc-500">
      The Code tool is not installed yet.
    </div>
  );
}

const CodePage = lazy(() =>
  codeLoader
    ? codeLoader().then((m) => ({ default: m.CodePage ?? CodeUnavailable }))
    : Promise.resolve({ default: CodeUnavailable }),
);

export default function App() {
  return (
    <div className="h-full font-sans antialiased">
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/chat" element={<ChatWindow />} />
        {/* The open project's id lives in the URL so reload/back restores it. */}
        <Route path="/builder" element={<Builder />} />
        <Route path="/builder/:projectId" element={<Builder />} />
        <Route
          path="/code"
          element={
            <Suspense fallback={<Loader />}>
              <CodePage />
            </Suspense>
          }
        />
        <Route
          path="/code/:projectId"
          element={
            <Suspense fallback={<Loader />}>
              <CodePage />
            </Suspense>
          }
        />
      </Routes>
    </div>
  );
}
