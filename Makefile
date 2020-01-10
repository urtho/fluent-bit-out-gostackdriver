all:
	go build -buildmode=c-shared -o out_gostackdriver.so

clean:
	rm -rf *.so *.h *~
