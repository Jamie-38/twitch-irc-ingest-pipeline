pipeline {
    agent any

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
            steps {
                sh 'go version'
                sh 'go test ./... -v'
            }
        }
    }
}