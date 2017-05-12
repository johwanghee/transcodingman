# transcodingman 설정

### 도커 이미지 빌드 (프로젝트 폴더 에서)
> sudo docker build -t transcodingman .

### 도커 실행
```
sudo docker run -it --rm \
    --name transcodingman \
    -p 5000:5000 \
    -v /home/incoming:/go/src/transcodingman/incoming \
    -v /home/outgoing:/go/src/transcodingman/outgoing \
    transcodingman
```
