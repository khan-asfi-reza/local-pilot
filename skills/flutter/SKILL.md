---
name: flutter
description: Flutter (Dart) app conventions.
internal: true
---
# Flutter (Dart)
- Everything is a Widget; compose Stateless/Stateful widgets. `build` returns the tree. Use `setState` (or the project's state lib) for updates.
- Deps in `pubspec.yaml`; run `flutter pub get`. Dart: sound null-safety, `final` by default.
- Verify: `flutter analyze` then `flutter test`. Don't try to boot an emulator headlessly.
