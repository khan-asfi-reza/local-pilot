---
name: cpp
description: C++ project conventions.
internal: true
---
# C++
- Prefer modern C++ (RAII, smart pointers, `std::` containers). Match the project's standard.
- Use the existing build (CMake `CMakeLists.txt` or Makefile); don't hand-invoke the compiler if a build exists.
- Verify: build with the project's system, then run. Compile with warnings and read them.
