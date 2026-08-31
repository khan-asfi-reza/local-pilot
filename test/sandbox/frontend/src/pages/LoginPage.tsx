import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { AuthForm } from '../components/AuthForm';
import { createV1AuthLogin } from '../lib/api';

export function LoginPage() {
  const navigate = useNavigate();
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (username: string, email: string | undefined, password: string) => {
    setIsLoading(true);
    try {
      const result = await createV1AuthLogin({ username, password });
      localStorage.setItem('token', result.token);
      navigate('/feed');
    } catch (error) {
      console.error('Login failed:', error);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="max-w-md w-full bg-white rounded-xl shadow-lg p-8">
        <h1 className="text-2xl font-bold text-gray-900 mb-2 text-center">Welcome Back</h1>
        <p className="text-gray-600 text-center mb-6">Sign in to continue to your feed</p>

        <AuthForm onSubmit={handleSubmit} onReset={() => {}} />

        <div className="mt-6 text-center">
          <p className="text-gray-600">
            Don't have an account?{' '}
            <Link to="/register" className="text-blue-600 hover:text-blue-700 font-medium">
              Sign up
            </Link>
          </p>
        </div>

        {isLoading && (
          <div className="mt-4 text-center text-gray-500 text-sm">Signing you in...</div>
        )}
      </div>
    </div>
  );
}
