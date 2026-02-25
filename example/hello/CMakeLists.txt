cmake_minimum_required(VERSION 3.10)
project(hello CXX)

add_executable(hello main.cpp)
install(TARGETS hello RUNTIME DESTINATION bin)
