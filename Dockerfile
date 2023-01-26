FROM ubuntu:latest

RUN apt-get update
RUN apt-get upgrade -y
RUN apt-get install git-core sudo build-essential cmake valgrind wget libcppunit-dev libunwind8 -y

RUN mkdir /drmem
WORKDIR /drmem
RUN wget https://github.com/DynamoRIO/drmemory/releases/download/release_2.5.0/DrMemory-Linux-2.5.0.tar.gz 
RUN tar xzf DrMemory-Linux-2.5.0.tar.gz
RUN export PATH=$PATH:'/drmem/DrMemory-Linux-2.5.0/bin'




WORKDIR /debugger

