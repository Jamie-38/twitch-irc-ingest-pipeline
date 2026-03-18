pipeline {
    agent any

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Inspect workspace in Go container') {
            agent {
                docker {
                    image 'golang:1.24'
                    reuseNode true
                }
            }
            steps {
                sh 'go version'
                sh 'pwd'
                sh 'ls -la'
                sh 'find . -maxdepth 2 -type f | sort | head -100'
            }
        }
    }
}