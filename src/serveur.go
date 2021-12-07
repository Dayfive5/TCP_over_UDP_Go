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
		fmt.Printf("\r [%2.0f%%] Waiting for ack #%d\n", 100*float64(*next_biggest_ack-1)/seq_max_float, *next_biggest_ack)
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

		nbseg := fi.Size()/1018
		if (nbseg*1018 < fi.Size()){
			nbseg = nbseg +1
		}
		fmt.Println(nbseg, "packet(s) to send")

		//chunk de données à envoyer
		chunkSize := 1018 

		//création d'un buffer
		packets := make([][]byte, nbseg)

		//On créé nos différents paquets dans une map
		for i:= 0; i<len(packets); i++ {
			packets[i]=make([]byte, chunkSize+6)
			
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
		next_biggest_ack := last_ack+1 //<=> dernier plus grand ack recu + 1
		winSize := 3
		seq_max := len(packets)

		send := func(num_seq int){
			//Si le numéro de séquence courant est inf ou = au numéro de séquence max
			if num_seq <= seq_max {
				//Si c'est le dernier paquet : on envoie que la partie remplie du paquet
				if num_seq == seq_max {
					n := (int)(fi.Size()) - (int)((seq_max-1)*(chunkSize))
					fmt.Println("Sending last packet number", num_seq)
					//n+6 car il faut rajouter le header
					_, err = conn.WriteToUDP(packets[num_seq-1][0:n+6], addr)
				} else {
					//Sinon on envoie le paquet
					fmt.Println("Sending packet number", num_seq)
					_, err = conn.WriteToUDP(packets[num_seq-1], addr)
				}
				//On set le timeout pour ce paquet
				timeouts[num_seq-1] = time.Now()
			}
		}

		window := func() bool {
			//Si le # du paquet courant est inf au dernier plus grand ack + 1
			if next_seq < next_biggest_ack {
				next_seq = next_biggest_ack
			}
			//On retourne true si le # de paquet courant inf à la taille de la fenetre + (dernier plus grand ack + 1)
			if next_seq < next_biggest_ack + winSize && (next_biggest_ack==1 || next_biggest_ack % winSize+1 ==0){
				return true
			} else {
				return false
			}
		}

		go func() {
			//tant que le dernier plus grand ack + 1 inf au # du dernier paquet
			for next_biggest_ack <= seq_max {
				fmt.Println("-----------F117-------------")
				fmt.Println("F117func next biggest ack", next_biggest_ack)
				fmt.Println("F117func next seq", next_seq)

				//On attend 1ms
				time.Sleep(time.Millisecond * 1)
				
				//Si notre paquet est OK
				if window() {
					//On l'envoie
					send(next_seq)
					fmt.Println("F117func send next seq", next_seq)

					//On passe au prochain paquet
					//if next_seq < seq_max {
					next_seq++
					
					//fmt.Println("next-seq:",next_seq)
				} else {
				//Sinon, si le temps de timeout du dernier + grand ack + 1 est supérieur à 900ms
					if time.Since(timeouts[next_biggest_ack])> time.Millisecond * 1500 {
						//Timeout -> On retransmet le paquet perdu
						fmt.Println("Timeout, retransmitting packet number", next_biggest_ack)
						send(next_biggest_ack)
					}
				}
			}

		}()
		//on affiche la progression en pourcentages de notre envoi
		//go progression(&next_biggest_ack, seq_max)

		//tant que le plus grand ack +1  inf au # du dernier paquet,
		for next_biggest_ack <= seq_max {
			//On lit l'ack recu
			_, _, err := conn.ReadFromUDP(buf)
			fmt.Println("ACK recu",string(buf))
			if err != nil {
				fmt.Println(err)
				return
			}
			//on récupère le numéro de séquence
			ack := getSeq(string(buf[3:9]))
			fmt.Println("------------F143-----------")
			fmt.Println("F143 ack:", ack)
			fmt.Println("F143last ack:", last_ack)
			fmt.Println("F143same ack:", same_ack)
			fmt.Println("F143next biggest ack:", next_biggest_ack)

			//Si c'est le meme ack qu'avant -> on incrémente same_ack
			if ack == last_ack {
				same_ack ++
				//A partir d'un certain nombre d'ack identiques recus, on renvoie le paquet perdu
				if same_ack > 0 {
					send(ack + 1)
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
func file(new_port int, addr *net.UDPAddr) {

	/*------OUVERTURE DE LA CONNEXION SUR LE NOUVEAU PORT------ */
	buffer := make([]byte, 1024)

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

	/*---------------RECUPERER LE NOM DU FICHIER---------------- */
	n, _, err := conn.ReadFromUDP(buffer)

	if err != nil {
		fmt.Println(err)
		return
	}

	buffer = buffer[:n-1]

	fileName := string(buffer)
	fmt.Println("Received message", n, "bytes:", fileName)

	/*--------------------ENVOYER LE FICHIER-------------------- */
	sendFile(conn, fileName, addr)
}

//La fonction add_conn fait le three-way handshake et attribue puis retourne le numéro de port pour l'envoi du fichier au client
//la fonction retourne le pointeur vers la connexion udp établie "conn"
func add_conn(addr *net.UDPAddr, buffer []byte, nbytes int, connection *net.UDPConn, new_port int) int {

	/*---------------------------------------------------------- */
	/*---------------------THREE-WAY HANDSHAKE------------------ */
	/*---------------------------------------------------------- */

	fmt.Println("-------------------------------------")
	fmt.Println("--------THREE-WAY HANDSHAKE----------")
	fmt.Println("-------------------------------------")

	//Si le message recu est un SYN
	if strings.Contains(string(buffer), "SYN") {

		fmt.Print("Received message ", nbytes, " bytes: ", string(buffer), "\n")
		fmt.Println("Sending SYN_ACK...")

		//Le serveur est pret : on envoie le SYN-ACK avec le nouveau port
		_, _ = connection.WriteToUDP([]byte("SYN-ACK"+strconv.Itoa(new_port)), addr)

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

}

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

	//On crée et initialise un objet buffer de type []byte et taille 1024
	buffer := make([]byte, 1024)
	new_port := 1024 //on commence à 1024 et pas 1000 car les 1024 sont limités pour les utilisateurs normaux (non root par exemple)

	//Création d'une map de connections ouvertes : clé = @ip:port_init ; valeur = new_port
	current_conn := make(map[*net.UDPAddr]int)

	for {
		//On lit le message recu et on le met dans le buffer
		nbytes, addr, err := connection.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println(err)
			return

		} else if _, found := current_conn[addr]; !found {
			/* si l'adresse de connexion n'est pas dans la map :
			- on ajoute l'adresse à la map
			- on lance la connexion avec la fonction add_conn */

			current_conn[addr] = new_port //clé: addr ; valeur = current_conn[addr]

			new_port += 1 //on incrémente le new_port de 1 pour la prochaine connexion

			if new_port == 9999 { //si on arrive à la fin de la plage de port, on reboucle au début de cette plage
				new_port = 1024
			}
			new_udp_port := add_conn(addr, buffer, nbytes, connection, current_conn[addr])
			// on lancera la goroutine avec l'envoi du fichier
			go file(new_udp_port, addr)
		}
	}
}
