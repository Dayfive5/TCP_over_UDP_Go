# TCP_over_UDP_Go
Set up TCP mechanisms above the UDP protocol in Go (PRS project)

### Goals
The purpose of this project is to design and implement servers that are adapted to 3 different scenarios. 
The scenario 1 uses one client of type client1.
The scenario 2 uses one client of type client2.
The scenario 3 uses several clients of type client1.
In all scenarios, the metric used to measure the performance of the servers will be the servers throughput.

The client1 & client2 files are located in the src folder.


### Organization
This repository contains the folder :
- *bin*/ -- contains 3 .exe files named serveurX-LesTryhardeusesDuDimanche, with X the number of the scenario
- *src*/ -- contains all the source files of the project and a Makefile that generates the 3 .exe files in bin
- *pres*/ -- contains the presentation of our project, in pdf

### Usage
To run the .exe server files you'll have to type in a terminal :
```
./serveurX-LesTryhardeusesDuDimanche <port number>
//X = <scenario number>
```
To run the .exe clients files you'll have to type in another terminal :
```
./clientX <IP server> <port number server> <file name>
//X = <client number>
```

To compile serveur.go you'll have to type in a terminal :
```
make
./serveur <port number>
```

To compile again serveur.go you'll have to type in a terminal :
```
make clean
make
./serveur <port number>
```
