/*

MIDI messages are comprised of two components: commands and data bytes.
The command byte tells the MIDI instrument what type of message is being
sent and the subsequent data byte(s) store the actual data. For example
a command byte might tell a MIDI instrument that it going to send information
about pitchbend, and the data byte describes how much pitchbend.

MIDI data bytes range from 0 to 127. Convert these numbers to binary and we
see they range from 00000000 to 01111111, the important thing to notice here
is that they always start with a 0 as the most significant bit (MSB). MIDI
command bytes range from 128 to 255, or 1000000 to 11111111 in binary. Unlike
data bytes, MIDI command bytes always start with a 1 as the MSB. This MSB is
how a MIDI instrument differentiates between a command byte and a data byte.

MIDI commands are further broken down by the following system:

The first half of the MIDI command byte (the three bits following the MSB) sets
the type of command. More info about the meaning on each of these commands is here.

10000000 = note off
10010000 = note on
10100000 = aftertouch
10110000 = continuous controller
11000000 = patch change
11010000 = channel pressure
11100000 = pitch bend
11110000 = non-musical commands

The last half of the command byte sets the MIDI channel. All the bytes
listed above would be in channel 0, command bytes ending in 0001 would
be for MIDI channel 1, and so on.

All MIDI messages start with a command byte, some messages contain one
data byte, others contain two or more (see image above). For example, a
note on command byte is followed by two data bytes: note and velocity.
I
'm going to explain how to use note on, note off, velocity, and pitchbend
in this instructable, since these are the most commonly used commands.
I'm sure you will be able to infer how to set up the others by the end of this.

*/

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

const (
	NoteOff         = 0x80
	NoteOn          = 0x90
	Aftertouch      = 0xa0
	ContinuousContr = 0xb0
	PatchChange     = 0xc0
	ChannelPressure = 0xD0
	PitchBend       = 0xE0
	SysExC          = 0xF0

	midiDevice  = "/dev/ttyAMA0"
	rumbaDevice = "/dev/ttyACM0"
	logFile     = "/tmp/midipump.log"
)

const (
	port = ":12101"
	udp  = `udp`

	midiCall = `/midi`
)

var (
	midiInDev  = flag.String("midi-in", "", "midi in device [/dev/snd/midi...]")
	midiOutDev = flag.String("midi-out", "", "midi out device [/dev/snd/midi...]")
	midiDev    = flag.String("midi", "", "midi in and out device [/dev/snd/midi...]")
)

type Midi struct {
	State    byte
	Channel  byte
	Note     byte
	Velocity byte
}

func ToMidi(req []byte) Midi {

	return Midi{
		State:    req[10] >> 4,
		Channel:  req[10] & 0x0f,
		Note:     req[9],
		Velocity: req[8],
	}
}

type MidiBridge struct {
	mu      sync.RWMutex
	MidiIn  *os.File
	MidiOut *os.File

	close chan bool
}

func NewMidiBridge(in, out *os.File) *MidiBridge {
	return &MidiBridge{

		MidiIn:  in,
		MidiOut: out,
	}
}

func (m *MidiBridge) Close() {
	m.close <- true
}

func (m *MidiBridge) Write(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MidiOut.Write(data)
}

func (m *MidiBridge) ListenMidiIn() {

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := m.MidiIn.Read(buf)
			if err != nil {
				log.Fatal(err)
			}
			bufCopy := make([]byte, n)
			copy(bufCopy, buf)
			m.handleDeviceIn(bufCopy)
		}
	}()

	for {
		select {
		case _, _ = <-m.close:
			return
		}
	}
}

func (m *MidiBridge) handleDeviceIn(req []byte) {
	fmt.Print("Midi Device In: ")
	for i := range req {
		fmt.Printf("%08b", req[i])
	}
	fmt.Println()
}

func (m *MidiBridge) handleBridgeIn(req []byte) {

	if len(req) != 11 {
		return
	}

	fmt.Printf("MidiNote: %+v\n", ToMidi(req))

	m.Write([]byte{req[10], req[9], req[8]})
}

func (m *MidiBridge) handleCmd(req []byte) {

	switch {
	case string(req[:len(midiCall)]) == midiCall:
		req := req[len(midiCall):]
		m.handleBridgeIn(req)

	default:
		fmt.Printf("%s not implemeted\n", req)
	}

}

func main() {

	flag.Parse()

	if *midiDev != "" {
		*midiInDev = *midiDev
		*midiOutDev = *midiDev
	}

	midiIn, err := os.OpenFile(*midiInDev, os.O_RDONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer midiIn.Close()
	midiOut, err := os.OpenFile(*midiOutDev, os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer midiOut.Close()

	bridge := NewMidiBridge(midiIn, midiOut)
	defer bridge.Close()

	go bridge.ListenMidiIn()

	udpSrv, err := net.ListenPacket(udp, port)
	if err != nil {
		log.Fatal(err)
	}
	defer udpSrv.Close()

	buf := make([]byte, 1024)

	for {

		n, addr, err := udpSrv.ReadFrom(buf)
		fmt.Println("Received ", string(buf[0:n]), " from ", addr)

		if err != nil {
			log.Println(err)
		}

		bufCopy := make([]byte, n)
		copy(bufCopy, buf)
		go bridge.handleCmd(bufCopy)
	}
}
