#/bin/bash
./client1 "$1" "$2" GigaFichier.txt&
./client1 "$1" "$2" GigaFichier2.txt&
./client1 "$1" "$2" GigaFichier3.txt&
./client1 "$1" "$2" GigaFichier4.txt&
./client1 "$1" "$2" GigaFichier5.txt&
wait
