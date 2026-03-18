pipeline {
    agent any

    environment {
        IMAGE_NAME = 'twitch-irc-ingest-pipeline'
        IMAGE_TAG = "${BUILD_NUMBER}"
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Run Go tests') {
            agent {
                docker {
                    image 'golang:1.24'
                    reuseNode true
                }
            }
            environment {
                HOME = "${WORKSPACE}"
                GOCACHE = "${WORKSPACE}/.gocache"
                GOMODCACHE = "${WORKSPACE}/.gomodcache"
            }
            steps {
                sh 'mkdir -p "$GOCACHE" "$GOMODCACHE"'
                sh 'go version'
                sh 'go test ./... -v'
            }
        }

        stage('Run golangci-lint') {
            agent {
                docker {
                    image 'golangci/golangci-lint:v1.64.8'
                    reuseNode true
                }
            }
            environment {
                HOME = "${WORKSPACE}"
                GOCACHE = "${WORKSPACE}/.gocache"
                GOMODCACHE = "${WORKSPACE}/.gomodcache"
            }
            steps {
                sh 'mkdir -p "$GOCACHE" "$GOMODCACHE"'
                sh 'golangci-lint version'
                sh 'golangci-lint run --timeout 5m'
            }
        }

        stage('Build Go binaries') {
            agent {
                docker {
                    image 'golang:1.24'
                    reuseNode true
                }
            }
            environment {
                HOME = "${WORKSPACE}"
                GOCACHE = "${WORKSPACE}/.gocache"
                GOMODCACHE = "${WORKSPACE}/.gomodcache"
            }
            steps {
                sh 'mkdir -p "$GOCACHE" "$GOMODCACHE" dist'

                sh '''
                    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
                    go build -o dist/irc_collector ./cmd/irc_collector
                '''

                sh '''
                    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
                    go build -o dist/oauth_server ./cmd/oauth_server
                '''

                sh '''
                    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
                    go build -o dist/kafka_consumer ./cmd/kafka_consumer
                '''

                sh 'ls -lah dist'
            }
        }

        stage('Build Docker image') {
            steps {
                sh 'mkdir -p dist'
                sh '''
                    docker build \
                      -t ${IMAGE_NAME}:${IMAGE_TAG} \
                      -t ${IMAGE_NAME}:latest \
                      .
                '''
                sh 'docker image ls | grep "${IMAGE_NAME}" || true'
                sh 'printf "%s:%s\\n" "${IMAGE_NAME}" "${IMAGE_TAG}" > dist/docker-image.txt'
                sh 'printf "%s:latest\\n" "${IMAGE_NAME}" >> dist/docker-image.txt'
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'dist/**', allowEmptyArchive: true
        }
    }
}