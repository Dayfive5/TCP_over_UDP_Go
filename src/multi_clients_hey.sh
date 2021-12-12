#/bin/bash
./client1 127.0.0.2 "$1" hey.txt
./client1 127.0.0.3 "$1" hey2.txt
./client1 127.0.0.4 "$1" hey3.txt
wait
