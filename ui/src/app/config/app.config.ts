export interface AppConfig {
  auth: {
    username: string;
    password: string;
  };
  acmeDns: {
    apiUrl: string;
    username: string;
    password: string;
  };
}

export const appConfig: AppConfig = {
  auth: {
    username: 'admin',
    password: 'admin123'
  },
  acmeDns: {
    apiUrl: 'https://acme-dns.netzint.de',
    username: '',
    password: ''
  }
};
