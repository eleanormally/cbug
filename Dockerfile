FROM ubuntu:latest
ARG GTEST_DIR=/usr/local/src/googletest/googletest

# Fix enter timezone issue
ENV TZ=Europe/Helsinki
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

RUN apt-get update
RUN apt-get upgrade -y
RUN apt-get install git-core sudo build-essential cmake valgrind libcppunit-dev libunwind8 -y

WORKDIR /bin
RUN curl https://github.com/DynamoRIO/drmemory/releases/download/release_2.5.0/DrMemory-Linux-2.5.0.tar.gz
RUN tar xzf DrMemory-Linux-2.5.0.tar.gz



# TODO: add Quick test, Boost test, Catch test

WORKDIR /debugger

# docker build -t heph3astus/cpp-memory-debugger:latest .
# docker run -tiv $PWD/path/to/my/files:/debugger heph3astus/cpp-memory-debugger:latest
