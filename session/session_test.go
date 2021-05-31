package session

import (
	"github.com/DarthPestilane/easytcp/packet"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
)

func TestSession_WaitToClose(t *testing.T) {
	r, _ := net.Pipe()
	sess := New(r, &packet.DefaultPacker{}, &packet.DefaultCodec{})
	go func() {
		<-time.After(time.Microsecond * 10)
		sess.Close()
	}()
	err := sess.WaitToClose()
	assert.NoError(t, err)
}

func TestSession_Close(t *testing.T) {
	r, _ := net.Pipe()
	sess := New(r, &packet.DefaultPacker{}, &packet.DefaultCodec{})
	for i := 0; i < 10; i++ {
		assert.NotPanics(t, func() {
			sess.Close() // goroutine safe
		})
	}
	_, ok := <-sess.closed
	assert.False(t, ok)
	_, ok = <-sess.ackQueue
	assert.False(t, ok)
	_, ok = <-sess.reqQueue
	assert.False(t, ok)
}

func TestSession_ReadLoop(t *testing.T) {
	packer := &packet.DefaultPacker{}
	codec := &packet.DefaultCodec{}

	data, err := codec.Encode("hello")
	assert.NoError(t, err)
	msg, err := packer.Pack(1, data)
	assert.NoError(t, err)

	r, w := net.Pipe()
	sess := New(r, packer, codec)
	go func() {
		_, _ = w.Write(msg) // send msg
	}()
	go sess.ReadLoop()

	req, ok := <-sess.RecvReq()
	assert.True(t, ok)
	assert.EqualValues(t, req.Id, 1)
	assert.Equal(t, req.RawData, []byte("hello"))

	sess.Close()
	err = sess.WaitToClose()
	assert.NoError(t, err)
}

func TestSession_WriteLoop(t *testing.T) {
	r, w := net.Pipe()
	packer := &packet.DefaultPacker{}
	codec := &packet.DefaultCodec{}
	sess := New(w, packer, codec)

	go sess.WriteLoop()

	err := sess.SendResp(&packet.Response{
		Id:   1,
		Data: "hello",
	})
	assert.NoError(t, err)
	msg, err := packer.Unpack(r) // read msg
	assert.NoError(t, err)
	assert.EqualValues(t, msg.GetId(), 1)
	assert.Equal(t, msg.GetData(), []byte("hello"))

	sess.Close()
	err = sess.WaitToClose()
	assert.NoError(t, err)
}
