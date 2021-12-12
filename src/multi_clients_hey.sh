#/bin/bash
./client1 "$1" "$2" hey.txt&
./client1 "$1" "$2" hey2.txt&
./client1 "$1" "$2" hey3.txt&
wait
