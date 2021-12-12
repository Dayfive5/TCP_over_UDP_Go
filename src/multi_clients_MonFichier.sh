#/bin/bash
./client1 127.0.0.2 "$1" MonFichier
./client1 127.0.0.3 "$1" MonFichier2
./client1 127.0.0.4 "$1" MonFichier3
wait
