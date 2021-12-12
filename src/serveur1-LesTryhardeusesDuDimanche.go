package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

/*-------------------------------------------------------------- */
/*--------------------------FONCTIONS--------------------------- */
/*-------------------------------------------------------------- */

func progression(next_biggest_ack *int, seq_max int) {
	//affiche le pourcentage d'avancement toutes les 100ms
	seq_max_float := float64(seq_max)
	for *next_biggest_ack-1 < seq_max {
		time.Sleep(time.Millisecond * 100)
		fmt.Printf("\r [%2.0f%%] #%d\n", 100*float64(*next_biggest_ack-1)/seq_max_float, *next_biggest_ack-1)
	}
}

func getSeq(ack string) (seq int) {
	fmt.Sscanf(ack, "%06d", &seq)
	return seq
}

func sendFile(conn *net.UDPConn, fileName string, addr *net.UDPAddr) {

	//On ouvre notre fichier
	var file, err = os.Open(fileName)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer file.Close()

	//Si le fichier n'est pas vide
	if file != nil {
		//On cherche la taille du fichier
		fi, err := file.Stat()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("The file is", fi.Size(), "bytes long")

		//chunk de données à envoyer
		chunkSize := 1494

		nbseg := int(fi.Size()) / chunkSize
		if nbseg*chunkSize < int(fi.Size()) {
			nbseg = nbseg + 1
		}
		fmt.Println(nbseg, "packet(s) to send")

		//création d'un buffer
		packets := make([][]byte, nbseg)

		//On créé nos différents paquets dans une map
		for i := 0; i < len(packets); i++ {
			packets[i] = make([]byte, chunkSize+6)

			//on ajoute le header en rajoutant les 0 nécessaires
			copy(packets[i][0:6], fmt.Sprintf("%06d", i+1))

			//on ajoute le chunk de données
			_, _ = file.Read(packets[i][6:])
		}

		//création de nos variables
		timeouts := make([]time.Time, len(packets)+2) //+2 sinon index out of range
		buf := make([]byte, 32)
		next_seq := 1
		last_ack := 0
		same_ack := 0
		lost_ack := false
		borneInfSlide := false
		borneInf := 1
		borneSup := 0
		next_biggest_ack := last_ack + 1 //<=> dernier plus grand ack recu + 1
		winSize := 65
		seq_max := len(packets)

		send := func(num_seq int) {
			//Si le numéro de séquence courant est inf ou = au numéro de séquence max
			if num_seq <= seq_max {
				//Si c'est le dernier paquet : on envoie que la partie remplie du paquet
				if num_seq == seq_max {
					n := (int)(fi.Size()) - (int)((seq_max-1)*(chunkSize))
					//fmt.Println("Sending last packet number", num_seq)
					//n+6 car il faut rajouter le header
					_, err = conn.WriteToUDP(packets[num_seq-1][0:n+6], addr)
				} else {
					//Sinon on envoie le paquet
					//fmt.Println("Sending packet number", num_seq)
					_, err = conn.WriteToUDP(packets[num_seq-1], addr)
				}
				//On set le timeout pour ce paquet
				timeouts[num_seq-1] = time.Now()
			}
		}

		window := func() bool {
			//Si le # du prochain paquet est inférieur au dernier plus grand ack + 1
			if next_seq < next_biggest_ack {
				next_seq = next_biggest_ack
			}

			//on calcule le quotient ENTIER du nba-1 par le winSize
			quotient := (next_biggest_ack - 1) / winSize

			if lost_ack == true {
				borneInf = next_biggest_ack
				lost_ack = false
				borneInfSlide = true
			} else {
				//Si la borne Inf n'a pas ete slidée
				if borneInfSlide == false {
					//on calcule la borne inférieure de la fenêtre en multipliant le quotient par le winSize et en ajoutant 1
					borneInf = (quotient * winSize) + 1

				} else { //la borne a été slidée
					if borneSup < next_biggest_ack { //le next_biggest_ack devient supérieur à la borne Sup
						borneInf = borneInf + winSize //on change alors la borne inférieure
					}
				}
			}

			//on calcule la borne supérieure de la fenêtre en ajoutant winSize-1 à la borne inf
			borneSup = (borneInf + winSize - 1)
			//fmt.Printf("next_biggest_ack =%d borneInf=%d borneSup=%d\n", next_biggest_ack, borneInf, borneSup)

			//On retourne true si le # de paquet courant est compris dans les bornes de la fenêtre en cours
			if (next_seq >= borneInf) && (next_seq <= borneSup) {
				return true
			} else {
				return false
			}
		}

		go func() {
			//tant que le dernier plus grand ack + 1 inf au # du dernier paquet
			for next_biggest_ack <= seq_max {
				//fmt.Println("-----------F117-------------")
				//fmt.Println("F117func next biggest ack", next_biggest_ack)
				//fmt.Println("F117func next seq", next_seq)

				//On attend 1ms
				time.Sleep(time.Millisecond * 1)

				//Si notre paquet est OK
				if window() {
					//On l'envoie
					send(next_seq)
					//fmt.Println("F117func send next seq", next_seq)

					//On passe au prochain paquet
					//if next_seq < seq_max {
					next_seq++

					//fmt.Println("next-seq:",next_seq)
				} else {
					//Sinon, si le temps de timeout du dernier + grand ack + 1 est supérieur à 300ms
					//pour etre sur que le nba n'a pas change entre temps

					if time.Since(timeouts[next_biggest_ack]) > time.Millisecond*300 {
						//Timeout -> On retransmet le paquet perdu
						//fmt.Println("Timeout, retransmitting packet number", next_biggest_ack)
						//fmt.Println("Timeout: next_seq:", next_seq)
						next_seq = next_biggest_ack

					}
				}
			}

		}()
		//on affiche la progression en pourcentages de notre envoi
		go progression(&next_biggest_ack, seq_max)

		//tant que le plus grand ack +1  inf au # du dernier paquet,
		for next_biggest_ack <= seq_max {
			//On lit l'ack recu
			_, _, err := conn.ReadFromUDP(buf)
			//fmt.Println("ACK recu",string(buf))
			if err != nil {
				fmt.Println(err)
				return
			}
			//on récupère le numéro de séquence
			ack := getSeq(string(buf[3:9]))
			//fmt.Println("------------F143-----------")
			//fmt.Println("F143 ack:", ack)
			//fmt.Println("F143last ack:", last_ack)
			//fmt.Println("F143same ack:", same_ack)
			//fmt.Println("F143next biggest ack:", next_biggest_ack)

			//Si c'est le meme ack qu'avant -> on incrémente same_ack
			if ack == last_ack {
				same_ack++
				//A partir d'un certain nombre d'ack identiques recus, on renvoie le paquet perdu
				//Fast retransmit
				if same_ack > 2 {
					next_seq = ack + 1
					lost_ack = true
					same_ack = 0
				}
			}
			//si l'ack est plus grand ou = à celui d'avant, il devient last_ack
			if ack >= last_ack {
				last_ack = ack
			}

			//Si l'ack est plus grand que le dernier plus grand ack recu +1, on met à jour ce dernier
			if last_ack >= next_biggest_ack {
				next_biggest_ack = last_ack + 1
			}

			//Fin de l'envoi : on envoie "FIN" au client
			if last_ack == seq_max {
				fmt.Println("End of transfer")
				_, err = conn.WriteToUDP([]byte("FIN"), addr)
			}
		}

	}
}

// La goroutine file gère les échanges client-serveur en lien avec le fichier en parallèle
func file(connStruct net.UDPConn, addrStruct net.UDPAddr) {

	fmt.Println("In file function ", addrStruct)
	buffer := make([]byte, 1024)

	/*---------------RECUPERER LE NOM DU FICHIER---------------- */
	n, _, err := (&connStruct).ReadFromUDP(buffer)

	if err != nil {
		fmt.Println(err)
		return
	}

	buffer = buffer[:n-1]

	fileName := string(buffer)
	fmt.Println("Received message", n, "bytes:", fileName)

	/*--------------------ENVOYER LE FICHIER-------------------- */
	sendFile(&connStruct, fileName, &addrStruct)
}

//La fonction add_conn fait le three-way handshake et attribue puis retourne le numéro de port pour l'envoi du fichier au client
//la fonction retourne le pointeur vers la connexion udp établie "conn"
/*func add_conn(addr *net.UDPAddr, buffer []byte, nbytes int, connection *net.UDPConn, new_port int) int {
		//On attend un ACK
		nbytes, _, err := connection.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println(err)
			return -1
		}

		if strings.Contains(string(buffer), "ACK") {
			fmt.Println("Received message", nbytes, "bytes :", string(buffer))
			fmt.Println("Three-way handshake established !")
			fmt.Println("-------------------------------------")
			return new_port
		}

	}
	return -1

}*/

/*-------------------------------------------------------------- */
/*-----------------------------MAIN----------------------------- */
/*-------------------------------------------------------------- */

func main() {
	/*---------------------------------------------------------- */
	/*-----------------------INITIALISATION--------------------- */
	/*---------------------------------------------------------- */

	//On récupère le port
	arguments := os.Args
	if len(arguments) < 2 {
		fmt.Println("Usage : ./serveur <port>")
		return
	}
	if len(arguments) > 2 {
		fmt.Println("Usage : ./serveur <port>")
		return
	}
	PORT := ":" + arguments[1]

	//On récupère l'adresse de l'UDP endpoint (endpoint=IP:port)
	s, err := net.ResolveUDPAddr("udp4", PORT)
	//fmt.Println("ResolveUDPAddr :", s)
	if err != nil {
		fmt.Println(err)
		return
	}
	//On créé un serveur UDP
	connection, err := net.ListenUDP("udp4", s)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer connection.Close()

	//On crée et initialise un objet buffer de type []byte et taille 1500
	buffer := make([]byte, 1500)
	new_port := 1024 //on commence à 1024 et pas 1000 car les 1024 sont limités pour les utilisateurs normaux (non root par exemple)

	//Création d'une map de connections ouvertes : clé = @ip:port_init ; valeur = new_port
	current_conn := make(map[int]*net.UDPConn)

	for {
		//fmt.Println("current_conn : ", current_conn)

		//On lit le message recu et on le met dans le buffer
		nbytes, addr, err := connection.ReadFromUDP(buffer)
		fmt.Println("adresse addr", addr)
		fmt.Println("Buffer, ", string(buffer))
		if err != nil {
			fmt.Println(err)
			return

		} else if _, found := current_conn[addr.Port]; !found {
			fmt.Println("not good: ", addr)
			/* si l'adresse de connexion n'est pas dans la map :
			- on ajoute l'adresse à la map
			- on lance la connexion avec la fonction add_conn */
			if strings.Contains(string(buffer), "SYN") {

				//current_conn[addr.Port] = new_port //clé: addr ; valeur = current_conn[addr]

				//new_udp_port := add_conn(addr, buffer, nbytes, connection, current_conn[addr])
				fmt.Println("-------------------------------------")
				fmt.Println("--------THREE-WAY HANDSHAKE----------")
				fmt.Println("-------------------------------------")

				//Si le message recu est un SYN

				fmt.Print("Received message ", nbytes, " bytes: ", string(buffer), "\n")
				fmt.Println("Sending SYN_ACK...")

				/*------OUVERTURE DE LA CONNEXION SUR LE NOUVEAU PORT------ */

				add, err := net.ResolveUDPAddr("udp4", (":" + strconv.Itoa(new_port)))
				if err != nil {
					fmt.Println(err)
					return
				}

				conn, err := net.ListenUDP("udp4", add)
				if err != nil {
					fmt.Println(err)
					return
				}

				defer conn.Close()
				current_conn[addr.Port] = conn
				//Le serveur est pret : on envoie le SYN-ACK avec le nouveau port
				_, _ = connection.WriteToUDP([]byte("SYN-ACK"+strconv.Itoa(new_port)), addr)

				new_port += 1 //on incrémente le new_port de 1 pour la prochaine connexion

				if new_port == 9999 { //si on arrive à la fin de la plage de port, on reboucle au début de cette plage
					new_port = 1024
				}
			}

		} else if strings.Contains(string(buffer), "ACK") { //si au contraire, paquet deja reçu depuis cette adresse

			fmt.Println("Received message", nbytes, "bytes :", string(buffer))
			fmt.Println("Three-way handshake established !")
			fmt.Println("-------------------------------------")

			// on lancera la goroutine avec l'envoi du fichier
			go file(*current_conn[addr.Port], *addr)
		}

	}

}
