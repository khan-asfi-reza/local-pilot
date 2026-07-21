---
name: react-native
description: React Native app conventions.
internal: true
---
# React Native
- Use RN core components (`View`, `Text`, `Pressable`, `FlatList`) — not web DOM elements. Style with `StyleSheet.create`.
- Detect Expo (`app.json`/`expo` dep) vs bare RN and follow it. Keep deps in `package.json`.
- No web-only APIs; use RN/Expo equivalents. Navigation via the project's library (React Navigation).
- Verify: type-check / `npm run build` if defined; don't try to boot a simulator headlessly.
