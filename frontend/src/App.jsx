import { Routes, Route } from 'react-router-dom';
import { Home } from './features/home/Home';
import { ChatWindow } from './components/chat/ChatWindow';
import { Builder } from './features/builder/Builder';

export default function App() {
  return (
    <div className="h-full font-sans antialiased">
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/chat" element={<ChatWindow />} />
        <Route path="/builder" element={<Builder />} />
        <Route path="/code" element={<ChatWindow />} />
      </Routes>
    </div>
  );
}
