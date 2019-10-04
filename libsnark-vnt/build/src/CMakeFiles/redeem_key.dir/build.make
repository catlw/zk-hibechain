# CMAKE generated file: DO NOT EDIT!
# Generated by "Unix Makefiles" Generator, CMake Version 3.5

# Delete rule output on recipe failure.
.DELETE_ON_ERROR:


#=============================================================================
# Special targets provided by cmake.

# Disable implicit rules so canonical targets will work.
.SUFFIXES:


# Remove some rules from gmake that .SUFFIXES does not remove.
SUFFIXES =

.SUFFIXES: .hpux_make_needs_suffix_list


# Suppress display of executed commands.
$(VERBOSE).SILENT:


# A target that is always out of date.
cmake_force:

.PHONY : cmake_force

#=============================================================================
# Set environment variables for the build.

# The shell in which to execute make rules.
SHELL = /bin/sh

# The CMake executable.
CMAKE_COMMAND = /usr/bin/cmake

# The command to remove a file.
RM = /usr/bin/cmake -E remove -f

# Escaping for special characters.
EQUALS = =

# The top-level source directory on which CMake was run.
CMAKE_SOURCE_DIR = /home/liubuntu/gopath/src/github.com/libsnark-vnt

# The top-level build directory on which CMake was run.
CMAKE_BINARY_DIR = /home/liubuntu/gopath/src/github.com/libsnark-vnt/build

# Include any dependencies generated for this target.
include src/CMakeFiles/redeem_key.dir/depend.make

# Include the progress variables for this target.
include src/CMakeFiles/redeem_key.dir/progress.make

# Include the compile flags for this target's objects.
include src/CMakeFiles/redeem_key.dir/flags.make

src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o: src/CMakeFiles/redeem_key.dir/flags.make
src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o: ../src/redeem/getpvk.cpp
	@$(CMAKE_COMMAND) -E cmake_echo_color --switch=$(COLOR) --green --progress-dir=/home/liubuntu/gopath/src/github.com/libsnark-vnt/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building CXX object src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o"
	cd /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src && /usr/bin/c++   $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -o CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o -c /home/liubuntu/gopath/src/github.com/libsnark-vnt/src/redeem/getpvk.cpp

src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color --switch=$(COLOR) --green "Preprocessing CXX source to CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.i"
	cd /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src && /usr/bin/c++  $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/liubuntu/gopath/src/github.com/libsnark-vnt/src/redeem/getpvk.cpp > CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.i

src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color --switch=$(COLOR) --green "Compiling CXX source to assembly CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.s"
	cd /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src && /usr/bin/c++  $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/liubuntu/gopath/src/github.com/libsnark-vnt/src/redeem/getpvk.cpp -o CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.s

src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.requires:

.PHONY : src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.requires

src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.provides: src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.requires
	$(MAKE) -f src/CMakeFiles/redeem_key.dir/build.make src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.provides.build
.PHONY : src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.provides

src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.provides.build: src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o


# Object files for target redeem_key
redeem_key_OBJECTS = \
"CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o"

# External object files for target redeem_key
redeem_key_EXTERNAL_OBJECTS =

src/redeem_key: src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o
src/redeem_key: src/CMakeFiles/redeem_key.dir/build.make
src/redeem_key: depends/libsnark/libsnark/libsnark.so
src/redeem_key: depends/libsnark/depends/libff/libff/libff.so
src/redeem_key: /usr/lib/x86_64-linux-gnu/libgmp.so
src/redeem_key: /usr/lib/x86_64-linux-gnu/libgmp.so
src/redeem_key: /usr/lib/x86_64-linux-gnu/libgmpxx.so
src/redeem_key: src/CMakeFiles/redeem_key.dir/link.txt
	@$(CMAKE_COMMAND) -E cmake_echo_color --switch=$(COLOR) --green --bold --progress-dir=/home/liubuntu/gopath/src/github.com/libsnark-vnt/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_2) "Linking CXX executable redeem_key"
	cd /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src && $(CMAKE_COMMAND) -E cmake_link_script CMakeFiles/redeem_key.dir/link.txt --verbose=$(VERBOSE)

# Rule to build all files generated by this target.
src/CMakeFiles/redeem_key.dir/build: src/redeem_key

.PHONY : src/CMakeFiles/redeem_key.dir/build

src/CMakeFiles/redeem_key.dir/requires: src/CMakeFiles/redeem_key.dir/redeem/getpvk.cpp.o.requires

.PHONY : src/CMakeFiles/redeem_key.dir/requires

src/CMakeFiles/redeem_key.dir/clean:
	cd /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src && $(CMAKE_COMMAND) -P CMakeFiles/redeem_key.dir/cmake_clean.cmake
.PHONY : src/CMakeFiles/redeem_key.dir/clean

src/CMakeFiles/redeem_key.dir/depend:
	cd /home/liubuntu/gopath/src/github.com/libsnark-vnt/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/liubuntu/gopath/src/github.com/libsnark-vnt /home/liubuntu/gopath/src/github.com/libsnark-vnt/src /home/liubuntu/gopath/src/github.com/libsnark-vnt/build /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src /home/liubuntu/gopath/src/github.com/libsnark-vnt/build/src/CMakeFiles/redeem_key.dir/DependInfo.cmake --color=$(COLOR)
.PHONY : src/CMakeFiles/redeem_key.dir/depend
