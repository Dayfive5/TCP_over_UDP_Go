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

/* Thibaud
 ****************
 * INTRODUCTION *
 ****************
 * Pour rendre l'envoi et la réception asynchrones et donc pouvoir envoyer les paquest par fenêtre,
 * il faut d'abord modifier la fonction sendFile pour découpler l'envoi et la réception des paquets.
 * Ici, le chargement du paquet, l'envoi du paquet, la réception de l'aquittement, le renvoi du paquet en cas d'éhec et enfin le vidage du buffer est synchrone.
 * Tout doit être asynchrone, donc pouvant être parallélisé afin d'améliorer les performances.
 * À faire :
 * 	1. Utiliser une map pour créer une file _toSend de paquets déjà préparé pour l'envoi. Chaque élement de la structure contiendra le numéro du paquet ainsi que le segment à envoyer
 *  2. Paralléliser la préparation de la file des segmnents et l'envoi des paquets
 *  3. Vider de la file dès qu'un paquet est envoyé pour le placer dans une autre file _sent (cette file est comme _toSend, une map id:segment)
 *  4. Changer le fonctionnement du handle pour replacer les paquets sans ack dans une troisième file _toResend (pareil que les autres, une map id:segment)
 *	5. Vider de la file _sent et les paquets bien reçus
 * Normalement pour faire cela, il faut 2/3 goroutines, une pour l'envoi et une pour la gestion des ack et peut être une pour le chargement du fichier dans la file _toSend. Ces goroutines doivent discuter entres elle pour prévenir quand un ack n'est pas reçu
 * Le plus importamt pour les performances est la partie 2-5 c'est à dire paralléliser réception et envoi, la parallélisation de la préparation des paquets a moins d'impacts
 *
 **************
 * PSEUDOCODE *
 **************
 * 1. func loadToBuffer(fileName string) map[int][]byte => charge le fichier dans la file _toSend
 *
 * 2-3. func sendFile(conn *net.UDPConn, fileName string, addr *net.UDPAddr, _toSend *map[int][]byte, _toResend *map[int][]byte) =>  Pour chaque segment dans _toResend ou _toSend (si _toResend vide) envoie un paquet et le place dans _sent. Attention à gérer le cas si _toSend est aussi vide car pas remplis assez vite et à ne pas trop envoyer de paquets si _sent est plein
 *
 * 4-5. func handleAck(conn *net.UDPConn, header string, addr *net.UDPAddr, _sent *map[int][]byte)) => Pour chaque segment dans _sent, vérifie si l'ack est reçu. si oui vide le segment de _sent, sinon rajoute le segment dans _toResend.
 *
 * En parralélisant, vous ferez face au problème de producer-comsummer pour _toSend, _sent et _toResend. Voici 2 liens pour le résoudre en Go:  https://www.golangprograms.com/illustration-of-producer-consumer-problem-in-golang.html, https://betterprogramming.pub/hands-on-go-concurrency-the-producer-consumer-pattern-c42aab4e3bd2.
 * Les fonctions du dessus sont là pour illustrer le fonctionnement, les paramètres sont faux car vous devrez utiliser des channels. Les éventuels return ne sont pas présents non plus.
 * Pour ajouter les fenêtres, le même fonctionnement est gardé mais on envoie par salve de la taille de la fenêtre au lieu de faire 1 par 1. Autrement dit, sendFile attends que _toSend ou _toResend ait une taille de n avant d'envoyer n paquets en même temps.
 *
 **********
 * ASTUCE *
 **********
 * Pour augmenter un peu la vitesse de transfert, le chunk size peut être augmenté vers 1500 bytes sans trop de soucis, vous réduirez donc le nombre de paquet à envoyer, mais en augmentant le taux d'erreur. Il vaut mieux tester mais les normes ethernet sont à 1500 bytes de payload donc normalement c'est bon.
 */

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
		fmt.Println("File opened")
		//On cherche la taille du fichier
		fi, err := file.Stat()
		if err != nil {
			fmt.Println(err)
			return
		}
		bufferSize := fi.Size()
		fmt.Println("The file is", fi.Size(), "bytes long")

		//On créé un buffer pour mettre le contenu du fichier dedans
		bufferSlice := make([]byte, bufferSize)

		//On lit le fichier dans notre buffer
		nbytes, err := file.Read(bufferSlice)
		if err != nil {
			fmt.Println(err)
			return
		}

		/*On va diviser le buffer en différents segments :*/

		//taille de nos buffers
		segSize := 0      //taille du segment total (max 1024 octets)
		chunkSize := 1018 //chunk de données à envoyer

		//création d'un compteur pour compter nos paquets
		count := 0

		//création d'un décompte du nombre de bytes à envoyer
		countdown := nbytes

		//Début de la fragmentation :
		bytesToCopy := 0

		//toSend := make(map[string][]byte)
		sent := make(map[string][]byte)
		toResend := make(map[string][]byte)

		go handle(conn, addr, sent, toResend)

		//tant que le fichier n'est pas vide
		for countdown > 0 {
			if len(sent) < 10 && len(toResend) < 10 {
				if len(toResend) > 0 {
					// Mutex to prevent concurrent access
					for header := range toResend {
						_, err = conn.WriteToUDP(toResend[header], addr)
						sent[header] = toResend[header]
						delete(toResend, header)
					}
				} else {
					//création des buffers
					var header string
					seg := make([]byte, segSize) // La valeur ancienne de seg sera vidée par le garbage collector, pas besoin de vider manuellement
					chunk := make([]byte, chunkSize)

					//On choisit la quantité de données à envoyer
					if countdown > chunkSize {
						bytesToCopy = chunkSize
					} else {
						bytesToCopy = countdown
					}
					//fmt.Println("---------------------------------------------------")

					//On copie les données dans le chunk
					copy(chunk, bufferSlice[:bytesToCopy])

					//Si on a moins de bytes à copier que la taille du chunk, on réduit la taille du chunk
					if bytesToCopy < chunkSize {
						chunk = chunk[:bytesToCopy]
					}
					//fmt.Println("chunk :", string(chunk))
					//fmt.Println("---------------------------------------------------")

					//On supprime le chunk du buffer
					bufferSlice = bufferSlice[bytesToCopy:]
					//fmt.Println("buffer:" , string(bufferSlice))

					//On décrémente le countdown
					countdown = countdown - bytesToCopy

					//On choisit un ID à mettre dans le header pour le segment en rajoutant les 0 nécessaires
					count = count + 1
					if count < 10 {
						i := 0
						for i < 5 {
							header = header + "0"
							i++
						}
						//fmt.Println(header)
					} else if count < 100 {
						i := 0
						for i < 4 {
							header = header + "0"
							i++
						}
					} else if count < 1000 {
						i := 0
						for i < 3 {
							header = header + "0"
							i++
						}
					} else if count < 10000 {
						i := 0
						for i < 2 {
							header = header + "0"
							i++
						}
					} else if count < 100000 {
						i := 0
						for i < 1 {
							header = header + "0"
							i++
						}
					}

					header = header + strconv.Itoa(count)

					//On met le header dans le segment à envoyer
					seg = append(seg, header...)

					//On met le chunk de données dans le segment à envoyer
					seg = append(seg, chunk...)
					//fmt.Println(string(seg))

					// Mutex to prevent concurrent access
					sent[header] = seg
					_, err = conn.WriteToUDP(sent[header], addr)
				}
			}

			//On envoie le segment au client

			//Gestion de pertes de paquets
			//time.Sleep(1 * time.Second)

			//On reset nos buffers
			// header = header[:0]
			// seg = seg[:0]

		}
		//Fin de l'envoi : on envoit "FIN" au client
		_, err = conn.WriteToUDP([]byte("FIN"), addr)
	}
}

func handle(conn *net.UDPConn, addr *net.UDPAddr, sent map[string][]byte, toResend map[string][]byte) {
	//On créé un buffer vide capable de garder 5 segments windowSeg[]
	//windowSeg := make([]byte, 5120)
	//On append chaque segment a ce buffer
	//for timeout>0 :
	//si ACK reçu pas le bon
	//On prend le num de seq de l'ACK reçu (ex: 000001)
	//On parcours buffSeg
	//On prend les 6 premiers bytes du seg headerSeg
	//Si string.Contains(numACK, headerSeg)
	//On retransmet les paquets a partir de ce segment
	//Si ACK recu :
	//On vide buffSeg

	//Pour chaque paquet, on regarde le dernier ACK recu

	var header string

	//fmt.Println("contenu buffer", string(buffACK))

	defer conn.Close()
	timeout, _ := time.ParseDuration("1000ms")

	for {
		conn.SetReadDeadline(time.Now().Add(timeout))
		buffACK := make([]byte, 1024)
		n, _, err := conn.ReadFromUDP(buffACK)
		//GESTION DES ERREURS (timeout ou autre erreur)
		if err != nil {
			fmt.Println("Eror: ", err)
			if err, ok := err.(net.Error); ok && err.Timeout() {
				//si on arrive là, c'est qu'on a pas reçu le premier ACK donc on retransmet la fenetre
				fmt.Println("Packet lost -> retransmition")
				// Mutex to prevent concurrent access
				// for k, v := range sent {
				// 	toResend[k] = v
				// 	delete(sent, k)
				// }
			}
		} else if buffACK != nil {
			fmt.Println("Received message :", string(buffACK), n, "bytes")
			//Si c'est le numero du header actuel -> continue
			header = string(buffACK[3:9])
			// Mutex to prevent concurrent access
			if _, ok := sent[header]; ok {
				//Tout va bien

				delete(sent, header)
			} else {
				var count int
				count = 1
				header = ""
				if count < 10 {
					i := 0
					for i < 5 {
						header = header + "0"
						i++
					}
					//fmt.Println(header)
				} else if count < 100 {
					i := 0
					for i < 4 {
						header = header + "0"
						i++
					}
				} else if count < 1000 {
					i := 0
					for i < 3 {
						header = header + "0"
						i++
					}
				} else if count < 10000 {
					i := 0
					for i < 2 {
						header = header + "0"
						i++
					}
				} else if count < 100000 {
					i := 0
					for i < 1 {
						header = header + "0"
						i++
					}
				}
				header = header + strconv.Itoa(count)
				// Mutex to prevent concurrent access
				toResend[header] = sent[header]
				delete(sent, header)

			}

			// 	} else {
			// 		//Sinon on prend le numero de l'ACK et on retransmet
			// 		//5 paquets a partir de ce numero d'ACK
			// 		_, err = conn.WriteToUDP(seg, addr)
			// 		buffACK = buffACK[:0]
			// 		handle(conn, header, addr, seg)
			// 		return

			// 	}
		}
	}

	// }

}

// La goroutine file gère les échanges client-serveur en lien avec le fichier en parallèle
func file(new_port int, addrStruct net.UDPAddr) {

	fmt.Println("In file function ", addrStruct)
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
	fmt.Println("In file function bef ", new_port)
	n, _, err := conn.ReadFromUDP(buffer)
	fmt.Println("In file function")

	if err != nil {
		fmt.Println(err)
		return
	}

	buffer = buffer[:n-1]
	fileName := string(buffer)
	fmt.Println("Received message", n, "bytes:", fileName)

	/*--------------------ENVOYER LE FICHIER-------------------- */
	sendFile(conn, fileName, &addrStruct)
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
	fmt.Println("ResolveUDPAddr :", s)
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
	current_conn := make(map[int]int)

	for {
		fmt.Println("current_conn : ", current_conn)

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

				current_conn[addr.Port] = new_port //clé: addr ; valeur = current_conn[addr]

				//new_udp_port := add_conn(addr, buffer, nbytes, connection, current_conn[addr])
				fmt.Println("-------------------------------------")
				fmt.Println("--------THREE-WAY HANDSHAKE----------")
				fmt.Println("-------------------------------------")

				//Si le message recu est un SYN

				fmt.Print("Received message ", nbytes, " bytes: ", string(buffer), "\n")
				fmt.Println("Sending SYN_ACK...")

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
			go file(current_conn[addr.Port], *addr)
		}

	}

}
